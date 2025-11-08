package tts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Engine TTS 引擎,管理多个提供商
type Engine struct {
	providers       map[string]Provider
	mu              sync.RWMutex
	defaultProvider Provider
	fallback        []Provider
}

// NewEngine 创建新的 TTS 引擎
func NewEngine(defaultProvider Provider) (*Engine, error) {
	engine := &Engine{
		providers:       make(map[string]Provider),
		defaultProvider: defaultProvider,
		fallback:        make([]Provider, 0),
	}
	if defaultProvider != nil {
		if err := engine.RegisterProvider(defaultProvider); err != nil {
			return nil, fmt.Errorf("failed to register default provider: %w", err)
		}
	}
	return engine, nil
}

// RegisterProvider 注册提供商
func (e *Engine) RegisterProvider(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	// 允许覆盖已注册的 provider
	// 如果需要禁止覆盖，可以检查是否已存在并返回错误
	// if _, exists := e.providers[name]; exists {
	//     return fmt.Errorf("provider %s already registered", name)
	// }

	e.providers[name] = provider
	return nil
}

// SetDefault 设置默认提供商
func (e *Engine) SetDefault(providerName string) error {
	e.mu.RLock()
	provider, ok := e.providers[providerName]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("provider %s not found", providerName)
	}

	e.mu.Lock()
	e.defaultProvider = provider
	e.mu.Unlock()
	return nil
}

// SetFallback 设置备用提供商列表(按优先级顺序)
func (e *Engine) SetFallback(providerNames []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	fallback := make([]Provider, 0, len(providerNames))
	for _, name := range providerNames {
		provider, ok := e.providers[name]
		if !ok {
			return fmt.Errorf("provider %s not found", name)
		}
		fallback = append(fallback, provider)
	}

	e.fallback = fallback
	return nil
}

// GetProvider 获取指定的提供商
func (e *Engine) GetProvider(name string) (Provider, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	provider, ok := e.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return provider, nil
}

// ListProviders 列出所有已注册的提供商
func (e *Engine) ListProviders() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.providers))
	for name := range e.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Synthesize 使用默认提供商合成语音
func (e *Engine) Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	return e.SynthesizeWithProvider(ctx, "", opts)
}

// SynthesizeWithProvider 使用指定提供商合成语音
func (e *Engine) SynthesizeWithProvider(ctx context.Context, providerName string, opts *SynthesizeOptions) ([]byte, error) {
	validatedOpts, err := validateOptions(opts)
	if err != nil {
		return nil, err
	}

	// 获取提供商
	var provider Provider
	if providerName != "" {
		p, err := e.GetProvider(providerName)
		if err != nil {
			return nil, err
		}
		provider = p
	} else {
		e.mu.RLock()
		provider = e.defaultProvider
		e.mu.RUnlock()
	}

	if provider == nil {
		return nil, fmt.Errorf("no provider available")
	}

	// 尝试合成
	audio, err := provider.Synthesize(ctx, validatedOpts)
	if err != nil {
		// 如果失败,尝试使用备用提供商
		if providerName == "" {
			return e.tryFallback(ctx, validatedOpts, provider.Name(), err)
		}
		return nil, &Error{
			Code:     ErrCodeInternalError,
			Message:  "synthesis failed",
			Provider: provider.Name(),
			Err:      err,
		}
	}

	return audio, nil
}

// SynthesizeStream 使用默认提供商流式合成语音
func (e *Engine) SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (AudioStream, error) {
	return e.SynthesizeStreamWithProvider(ctx, "", opts)
}

// SynthesizeStreamWithProvider 使用指定提供商流式合成语音
func (e *Engine) SynthesizeStreamWithProvider(ctx context.Context, providerName string, opts *SynthesizeOptions) (AudioStream, error) {
	validatedOpts, err := validateOptions(opts)
	if err != nil {
		return nil, err
	}

	// 获取提供商
	var provider Provider
	if providerName != "" {
		p, err := e.GetProvider(providerName)
		if err != nil {
			return nil, err
		}
		provider = p
	} else {
		e.mu.RLock()
		provider = e.defaultProvider
		e.mu.RUnlock()
	}

	if provider == nil {
		return nil, fmt.Errorf("no provider available")
	}

	return provider.SynthesizeStream(ctx, validatedOpts)
}

// ListVoices 列出所有提供商的可用语音
func (e *Engine) ListVoices(ctx context.Context, locale string) ([]Voice, error) {
	e.mu.RLock()
	providers := make([]Provider, 0, len(e.providers))
	for _, p := range e.providers {
		providers = append(providers, p)
	}
	e.mu.RUnlock()

	voices := make([]Voice, 0)
	for _, provider := range providers {
		if !provider.IsAvailable(ctx) {
			continue
		}
		pvs, err := provider.ListVoices(ctx, locale)
		if err != nil {
			continue // 忽略错误,继续其他提供商
		}
		voices = append(voices, pvs...)
	}

	return voices, nil
}

// ListVoicesForProvider 列出指定提供商的可用语音
func (e *Engine) ListVoicesForProvider(ctx context.Context, providerName, locale string) ([]Voice, error) {
	provider, err := e.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	return provider.ListVoices(ctx, locale)
}

// tryFallback 尝试使用备用提供商
func (e *Engine) tryFallback(ctx context.Context, opts *SynthesizeOptions, primaryProvider string, primaryErr error) ([]byte, error) {
	e.mu.RLock()
	fallback := e.fallback
	e.mu.RUnlock()

	// 记录所有失败
	failures := []string{fmt.Sprintf("%s: %v", primaryProvider, primaryErr)}

	for _, provider := range fallback {
		if !provider.IsAvailable(ctx) {
			continue
		}

		audio, err := provider.Synthesize(ctx, opts)
		if err == nil {
			return audio, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", provider.Name(), err))
	}

	return nil, &Error{
		Code:    ErrCodeProviderUnavail,
		Message: fmt.Sprintf("all providers failed: %s", strings.Join(failures, "; ")),
	}
}

// validateOptions 验证选项并返回修正后的副本
func validateOptions(opts *SynthesizeOptions) (*SynthesizeOptions, error) {
	if opts == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	if opts.Text == "" {
		return nil, &Error{
			Code:    ErrCodeInvalidInput,
			Message: "text cannot be empty",
		}
	}

	// 创建副本以避免修改原始对象
	validatedOpts := &SynthesizeOptions{
		Text:                    opts.Text,
		Voice:                   opts.Voice,
		Rate:                    opts.Rate,
		Volume:                  opts.Volume,
		Pitch:                   opts.Pitch,
		Locale:                  opts.Locale,
		WordBoundaryEnabled:     opts.WordBoundaryEnabled,
		SentenceBoundaryEnabled: opts.SentenceBoundaryEnabled,
		MetadataCallback:        opts.MetadataCallback,
		BackgroundMusic:         opts.BackgroundMusic,
		Extra:                   opts.Extra,
	}

	// 设置默认值
	if validatedOpts.Rate <= 0 {
		validatedOpts.Rate = 1.0
	}
	if validatedOpts.Volume <= 0 {
		validatedOpts.Volume = 1.0
	}
	if validatedOpts.Pitch <= 0 {
		validatedOpts.Pitch = 1.0
	}

	return validatedOpts, nil
}
