package tts

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Default FormatRegistry configuration values.
const (
	defaultProbeTTL      = 7 * 24 * time.Hour // 7 days
	defaultProbeInterval = 2 * time.Second
)

var constantVerifiedAt = time.Unix(1, 0).UTC()

// FormatStatus represents the verification state of an output format.
type FormatStatus int

const (
	// FormatUnverified indicates the format has been declared but not verified at runtime.
	FormatUnverified FormatStatus = iota
	// FormatAvailable indicates the format is verified available (or a compile-time constant).
	FormatAvailable
	// FormatUnavailable indicates the format was verified unavailable at runtime.
	FormatUnavailable
)

// OutputFormat describes a Provider-supported audio output format.
type OutputFormat struct {
	ID         string            // Format identifier (e.g. "audio-24khz-48kbitrate-mono-mp3")
	Profile    VoiceAudioProfile // Audio characteristics
	Status     FormatStatus      // Availability status
	VerifiedAt time.Time         // Last probe time (zero means never probed)
}

// FormatProber is an optional interface that a Provider can implement to support
// runtime format probing.
type FormatProber interface {
	// ProbeFormat tests whether the given format is available.
	// Returns true if the format produced valid audio data.
	ProbeFormat(ctx context.Context, formatID string) (bool, error)
}

// FormatRegistry is a thread-safe format registry that manages format
// declarations, probing, and caching. It is generic infrastructure applicable
// to all Providers.
type FormatRegistry struct {
	mu            sync.RWMutex
	formats       map[string]*OutputFormat
	prober        FormatProber
	parser        func(string) (VoiceAudioProfile, bool)
	probeTTL      time.Duration
	probeInterval time.Duration
}

// FormatRegistryOption configures a FormatRegistry.
type FormatRegistryOption func(*FormatRegistry)

// WithProber injects a format prober for runtime format verification.
func WithProber(p FormatProber) FormatRegistryOption {
	return func(r *FormatRegistry) {
		r.prober = p
	}
}

// WithProfileParser injects a format string parser that can derive
// VoiceAudioProfile from a format ID string. Used by Get() for auto-parsing
// unknown formats.
func WithProfileParser(fn func(string) (VoiceAudioProfile, bool)) FormatRegistryOption {
	return func(r *FormatRegistry) {
		r.parser = fn
	}
}

// WithProbeTTL sets the time-to-live for probe results. Default is 7 days.
func WithProbeTTL(d time.Duration) FormatRegistryOption {
	return func(r *FormatRegistry) {
		r.probeTTL = d
	}
}

// WithProbeInterval sets the delay between individual format probes in
// ProbeAll. Default is 2 seconds (to avoid rate limiting).
func WithProbeInterval(d time.Duration) FormatRegistryOption {
	return func(r *FormatRegistry) {
		r.probeInterval = d
	}
}

// NewFormatRegistry creates a FormatRegistry with the given options.
func NewFormatRegistry(opts ...FormatRegistryOption) *FormatRegistry {
	r := &FormatRegistry{
		formats:       make(map[string]*OutputFormat),
		probeTTL:      defaultProbeTTL,
		probeInterval: defaultProbeInterval,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds one or more formats to the registry in bulk.
// Existing entries with the same ID are overwritten.
func (r *FormatRegistry) Register(formats ...OutputFormat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range formats {
		f := formats[i] // copy
		r.formats[f.ID] = &f
	}
}

// RegisterConstant registers a format that is known to be available at compile
// time. The format is marked as FormatAvailable and requires no probing.
func (r *FormatRegistry) RegisterConstant(id string, profile VoiceAudioProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.formats[id] = &OutputFormat{
		ID:         id,
		Profile:    profile,
		Status:     FormatAvailable,
		VerifiedAt: constantVerifiedAt,
	}
}

// Get retrieves a format by ID. If the format is not registered but a parser
// is configured, it attempts to parse the ID and auto-register the result as
// FormatUnverified. Returns false if the format is unknown and cannot be parsed.
func (r *FormatRegistry) Get(formatID string) (*OutputFormat, bool) {
	// Fast path: read lock
	r.mu.RLock()
	if f, ok := r.formats[formatID]; ok {
		cp := *f
		r.mu.RUnlock()
		return &cp, true
	}
	parser := r.parser
	r.mu.RUnlock()

	if parser == nil {
		return nil, false
	}

	// Slow path: attempt auto-parse and register
	profile, ok := parser(formatID)
	if !ok {
		return nil, false
	}

	r.mu.Lock()
	// Double check: another goroutine may have registered it
	if f, ok := r.formats[formatID]; ok {
		cp := *f
		r.mu.Unlock()
		return &cp, true
	}
	f := &OutputFormat{
		ID:      formatID,
		Profile: profile,
		Status:  FormatUnverified,
	}
	r.formats[formatID] = f
	cp := *f
	r.mu.Unlock()
	return &cp, true
}

// Available returns all formats with FormatAvailable status, sorted by ID.
func (r *FormatRegistry) Available() []OutputFormat {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []OutputFormat
	for _, f := range r.formats {
		if f.Status == FormatAvailable {
			result = append(result, *f)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// All returns all registered formats regardless of status, sorted by ID.
func (r *FormatRegistry) All() []OutputFormat {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]OutputFormat, 0, len(r.formats))
	for _, f := range r.formats {
		result = append(result, *f)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Probe probes a single format and updates the registry with the result.
//
// Behavior:
//   - If the format is already FormatAvailable and the probe has not expired,
//     the cached result is returned without calling the prober.
//   - Otherwise, the prober is invoked and the result is stored in the registry.
//   - If the format is not yet registered, it is auto-registered.
//   - Returns an error if no prober is configured.
func (r *FormatRegistry) Probe(ctx context.Context, formatID string) (*OutputFormat, error) {
	r.mu.RLock()
	prober := r.prober
	parser := r.parser
	probeTTL := r.probeTTL
	var fCopy OutputFormat
	var found bool
	if fp := r.formats[formatID]; fp != nil {
		fCopy = *fp
		found = true
	}
	r.mu.RUnlock()

	if found && isConstantFormat(&fCopy) {
		return &fCopy, nil
	}

	// Return cached result if available and not expired
	if found && fCopy.Status == FormatAvailable && !fCopy.VerifiedAt.IsZero() &&
		time.Since(fCopy.VerifiedAt) < probeTTL {
		return &fCopy, nil
	}

	if prober == nil {
		return nil, errors.New("tts: no FormatProber configured")
	}

	// Invoke prober
	available, err := prober.ProbeFormat(ctx, formatID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	status := FormatUnavailable
	if available {
		status = FormatAvailable
	}

	r.mu.Lock()
	existing, ok := r.formats[formatID]
	if ok {
		existing.Status = status
		existing.VerifiedAt = now
		cp := *existing
		r.mu.Unlock()
		return &cp, nil
	}
	// Auto-register previously unknown format
	profile := VoiceAudioProfile{}
	if parser != nil {
		if parsed, parsedOK := parser(formatID); parsedOK {
			profile = parsed
		}
	}
	entry := &OutputFormat{
		ID:         formatID,
		Profile:    profile,
		Status:     status,
		VerifiedAt: now,
	}
	r.formats[formatID] = entry
	cp := *entry
	r.mu.Unlock()
	return &cp, nil
}

// ProbeAll probes all FormatUnverified formats in the registry.
// It returns counts of newly available and unavailable formats.
// Formats are probed sequentially with the configured probe interval to avoid
// rate limiting. Returns early if the context is cancelled.
func (r *FormatRegistry) ProbeAll(ctx context.Context) (available, unavailable int, err error) {
	// Snapshot unverified format IDs
	r.mu.RLock()
	var ids []string
	for id, f := range r.formats {
		if f.Status == FormatUnverified {
			ids = append(ids, id)
		}
	}
	interval := r.probeInterval
	r.mu.RUnlock()

	sort.Strings(ids) // deterministic order

	for i, id := range ids {
		// Respect context cancellation
		if err := ctx.Err(); err != nil {
			return available, unavailable, err
		}

		f, probeErr := r.Probe(ctx, id)
		if probeErr != nil {
			// Skip formats that fail to probe (e.g. prober error)
			unavailable++
			continue
		}

		switch f.Status {
		case FormatAvailable:
			available++
		default:
			unavailable++
		}

		// Apply interval between probes (skip after last)
		if interval > 0 && i < len(ids)-1 {
			select {
			case <-ctx.Done():
				return available, unavailable, ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return available, unavailable, nil
}

// SetProber sets or replaces the format prober. This supports lazy injection
// where the Provider instance is created after the registry.
func (r *FormatRegistry) SetProber(p FormatProber) {
	r.mu.Lock()
	r.prober = p
	r.mu.Unlock()
}

// HasProber reports whether a format prober is configured.
func (r *FormatRegistry) HasProber() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.prober != nil
}

// Clone creates a copy of the registry with the same formats, parser,
// probeTTL and probeInterval settings. The prober is NOT copied — the caller
// should set a prober on the clone via SetProber if needed.
func (r *FormatRegistry) Clone() *FormatRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c := &FormatRegistry{
		formats:       make(map[string]*OutputFormat, len(r.formats)),
		parser:        r.parser,
		probeTTL:      r.probeTTL,
		probeInterval: r.probeInterval,
	}
	for id, f := range r.formats {
		cp := *f
		c.formats[id] = &cp
	}
	return c
}

// IsProbeExpired reports whether a format's probe result has expired based on
// the registry's probeTTL. A format with a zero VerifiedAt is always considered
// expired.
func (r *FormatRegistry) IsProbeExpired(f *OutputFormat) bool {
	if isConstantFormat(f) {
		return false
	}
	if f.VerifiedAt.IsZero() {
		return true
	}
	r.mu.RLock()
	ttl := r.probeTTL
	r.mu.RUnlock()
	return time.Since(f.VerifiedAt) >= ttl
}

func isConstantFormat(f *OutputFormat) bool {
	if f == nil {
		return false
	}
	return f.Status == FormatAvailable && f.VerifiedAt.Equal(constantVerifiedAt)
}
