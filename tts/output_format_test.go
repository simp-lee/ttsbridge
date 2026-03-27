package tts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockProber implements FormatProber for testing.
type mockProber struct {
	mu      sync.Mutex
	results map[string]bool // formatID → available
	errs    map[string]error
	calls   []string // record of ProbeFormat calls
}

type nilCheckingProber struct {
	called bool
}

func newMockProber(results map[string]bool) *mockProber {
	return &mockProber{results: results}
}

func (m *mockProber) ProbeFormat(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	m.calls = append(m.calls, id)
	m.mu.Unlock()
	if err, ok := m.errs[id]; ok {
		return false, err
	}
	if avail, ok := m.results[id]; ok {
		return avail, nil
	}
	return false, fmt.Errorf("unknown format: %s", id)
}

func (m *mockProber) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (p *nilCheckingProber) ProbeFormat(ctx context.Context, id string) (bool, error) {
	p.called = true
	if ctx == nil {
		return false, errors.New("nil context")
	}
	if id != "fmt-nil" {
		return false, fmt.Errorf("id = %s; want fmt-nil", id)
	}
	return true, nil
}

func nilContextForFormatRegistryTest() context.Context {
	var ctx context.Context
	return ctx
}

func TestFormatRegistry_RegisterConstant(t *testing.T) {
	r := NewFormatRegistry()
	r.RegisterConstant("fmt-mp3", VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48})

	f, ok := r.Get("fmt-mp3")
	if !ok {
		t.Fatal("expected to find registered constant format")
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
	if f.Profile.Format != "mp3" {
		t.Errorf("profile.Format = %q; want %q", f.Profile.Format, "mp3")
	}
	if f.Profile.SampleRate != 24000 {
		t.Errorf("profile.SampleRate = %d; want 24000", f.Profile.SampleRate)
	}
}

func TestFormatRegistry_RegisterUnverified(t *testing.T) {
	r := NewFormatRegistry()
	r.Register(OutputFormat{
		ID:      "fmt-ogg",
		Profile: VoiceAudioProfile{Format: "ogg", SampleRate: 16000},
		Status:  FormatUnverified,
	})

	f, ok := r.Get("fmt-ogg")
	if !ok {
		t.Fatal("expected to find registered format")
	}
	if f.Status != FormatUnverified {
		t.Errorf("status = %v; want FormatUnverified", f.Status)
	}

	// Available() should not include unverified formats
	avail := r.Available()
	for _, a := range avail {
		if a.ID == "fmt-ogg" {
			t.Error("Available() should not include unverified formats")
		}
	}
}

func TestFormatRegistry_All(t *testing.T) {
	r := NewFormatRegistry()
	r.RegisterConstant("c1", VoiceAudioProfile{Format: "mp3"})
	r.Register(OutputFormat{ID: "u1", Status: FormatUnverified})
	r.Register(OutputFormat{ID: "u2", Status: FormatUnavailable})

	all := r.All()
	if len(all) != 3 {
		t.Errorf("All() returned %d formats; want 3", len(all))
	}
}

func TestFormatRegistry_Probe_Success(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-a": true})
	r := NewFormatRegistry(WithProber(prober))
	r.Register(OutputFormat{ID: "fmt-a", Status: FormatUnverified})

	f, err := r.Probe(context.Background(), "fmt-a")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
	if f.VerifiedAt.IsZero() {
		t.Error("VerifiedAt should be set after probe")
	}

	// Verify it's now in Available()
	avail := r.Available()
	found := false
	for _, a := range avail {
		if a.ID == "fmt-a" {
			found = true
		}
	}
	if !found {
		t.Error("fmt-a should appear in Available() after successful probe")
	}
}

func TestFormatRegistry_Probe_NilContextUsesBackground(t *testing.T) {
	prober := &nilCheckingProber{}
	r := NewFormatRegistry(WithProber(prober))
	r.Register(OutputFormat{ID: "fmt-nil", Status: FormatUnverified})

	f, err := r.Probe(nilContextForFormatRegistryTest(), "fmt-nil")
	if err != nil {
		t.Fatalf("Probe(nil, ...) error: %v", err)
	}
	if !prober.called {
		t.Fatal("expected prober to be called")
	}
	if f.Status != FormatAvailable {
		t.Fatalf("status = %v; want %v", f.Status, FormatAvailable)
	}
}

func TestFormatRegistry_Probe_Failure(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-b": false})
	r := NewFormatRegistry(WithProber(prober))
	r.Register(OutputFormat{ID: "fmt-b", Status: FormatUnverified})

	f, err := r.Probe(context.Background(), "fmt-b")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatUnavailable {
		t.Errorf("status = %v; want FormatUnavailable", f.Status)
	}
	if f.VerifiedAt.IsZero() {
		t.Error("VerifiedAt should be set even on unavailable probe")
	}
}

func TestFormatRegistry_Probe_AutoCache(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-c": true})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeTTL(1*time.Hour),
	)
	r.Register(OutputFormat{ID: "fmt-c", Status: FormatUnverified})

	// First probe
	_, err := r.Probe(context.Background(), "fmt-c")
	if err != nil {
		t.Fatalf("first Probe() error: %v", err)
	}

	// Second probe should use cache (no additional prober call)
	f, err := r.Probe(context.Background(), "fmt-c")
	if err != nil {
		t.Fatalf("second Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
	if prober.callCount() != 1 {
		t.Errorf("prober called %d times; want 1 (cached)", prober.callCount())
	}
}

func TestFormatRegistry_Probe_CachesUnavailableWithinTTL(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-unavailable": false})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeTTL(time.Hour),
	)
	r.Register(OutputFormat{ID: "fmt-unavailable", Status: FormatUnverified})

	first, err := r.Probe(context.Background(), "fmt-unavailable")
	if err != nil {
		t.Fatalf("first Probe() error: %v", err)
	}
	if first.Status != FormatUnavailable {
		t.Fatalf("first status = %v; want FormatUnavailable", first.Status)
	}

	second, err := r.Probe(context.Background(), "fmt-unavailable")
	if err != nil {
		t.Fatalf("second Probe() error: %v", err)
	}
	if second.Status != FormatUnavailable {
		t.Fatalf("second status = %v; want FormatUnavailable", second.Status)
	}
	if prober.callCount() != 1 {
		t.Fatalf("prober called %d times; want 1 for cached unavailable result", prober.callCount())
	}
}

func TestFormatRegistry_Options_IgnoreNonPositiveDurations(t *testing.T) {
	r := NewFormatRegistry(
		WithProbeTTL(0),
		WithProbeInterval(-1*time.Second),
	)

	if r.probeTTL != defaultProbeTTL {
		t.Fatalf("probeTTL = %v; want default %v", r.probeTTL, defaultProbeTTL)
	}
	if r.probeInterval != defaultProbeInterval {
		t.Fatalf("probeInterval = %v; want default %v", r.probeInterval, defaultProbeInterval)
	}

	r = NewFormatRegistry(
		WithProbeTTL(3*time.Second),
		WithProbeInterval(250*time.Millisecond),
	)

	if r.probeTTL != 3*time.Second {
		t.Fatalf("probeTTL = %v; want 3s", r.probeTTL)
	}
	if r.probeInterval != 250*time.Millisecond {
		t.Fatalf("probeInterval = %v; want 250ms", r.probeInterval)
	}
}

func TestFormatRegistry_Probe_ConstantBypassesProber(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-const": false})
	r := NewFormatRegistry(WithProber(prober))
	r.RegisterConstant("fmt-const", VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48})

	f, err := r.Probe(context.Background(), "fmt-const")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
	if prober.callCount() != 0 {
		t.Errorf("prober called %d times; want 0 for constant format", prober.callCount())
	}

	stored, ok := r.Get("fmt-const")
	if !ok {
		t.Fatal("expected constant format to remain registered")
	}
	if stored.Status != FormatAvailable {
		t.Errorf("stored status = %v; want FormatAvailable", stored.Status)
	}
}

func TestFormatRegistry_Probe_ConstantNoProber(t *testing.T) {
	r := NewFormatRegistry()
	r.RegisterConstant("fmt-const-no-prober", VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48})

	f, err := r.Probe(context.Background(), "fmt-const-no-prober")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
}

func TestFormatRegistry_Probe_NoProber(t *testing.T) {
	r := NewFormatRegistry() // no prober
	r.Register(OutputFormat{ID: "fmt-x", Status: FormatUnverified})

	_, err := r.Probe(context.Background(), "fmt-x")
	if err == nil {
		t.Fatal("expected error when no prober is set")
	}
}

func TestFormatRegistry_Probe_NotRegistered(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-new": true})
	r := NewFormatRegistry(WithProber(prober))

	// Probe an unregistered format — should auto-register
	f, err := r.Probe(context.Background(), "fmt-new")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
}

func TestFormatRegistry_Probe_NotRegisteredParseable_PreservesProfile(t *testing.T) {
	parser := func(id string) (VoiceAudioProfile, bool) {
		if id == "auto-probe-format" {
			return VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48}, true
		}
		return VoiceAudioProfile{}, false
	}
	prober := newMockProber(map[string]bool{"auto-probe-format": true})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProfileParser(parser),
	)

	f, err := r.Probe(context.Background(), "auto-probe-format")
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
	if f.Profile.Format != "mp3" || f.Profile.SampleRate != 24000 || f.Profile.Bitrate != 48 {
		t.Fatalf("profile = %+v; want parsed profile", f.Profile)
	}

	stored, ok := r.Get("auto-probe-format")
	if !ok {
		t.Fatal("expected auto-probed format to be registered")
	}
	if stored.Profile.Format != "mp3" || stored.Profile.SampleRate != 24000 || stored.Profile.Bitrate != 48 {
		t.Fatalf("stored profile = %+v; want parsed profile", stored.Profile)
	}
}

func TestFormatRegistry_ProbeAll(t *testing.T) {
	prober := newMockProber(map[string]bool{
		"f1": true,
		"f2": false,
		"f3": true,
	})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeInterval(1*time.Millisecond),
	)
	r.Register(
		OutputFormat{ID: "f1", Status: FormatUnverified},
		OutputFormat{ID: "f2", Status: FormatUnverified},
		OutputFormat{ID: "f3", Status: FormatUnverified},
	)
	// Also register a constant that should be skipped
	r.RegisterConstant("f0", VoiceAudioProfile{Format: "mp3"})

	available, unavailable, err := r.ProbeAll(context.Background())
	if err != nil {
		t.Fatalf("ProbeAll() error: %v", err)
	}
	if available != 2 {
		t.Errorf("available = %d; want 2", available)
	}
	if unavailable != 1 {
		t.Errorf("unavailable = %d; want 1", unavailable)
	}
	// Prober should have been called exactly 3 times (only unverified formats)
	if prober.callCount() != 3 {
		t.Errorf("prober called %d times; want 3", prober.callCount())
	}
}

func TestFormatRegistry_ProbeAll_ContextCancel(t *testing.T) {
	prober := newMockProber(map[string]bool{
		"f1": true,
		"f2": true,
	})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeInterval(1*time.Millisecond),
	)
	r.Register(
		OutputFormat{ID: "f1", Status: FormatUnverified},
		OutputFormat{ID: "f2", Status: FormatUnverified},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := r.ProbeAll(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestFormatRegistry_ProbeAll_NilContextUsesBackground(t *testing.T) {
	prober := newMockProber(map[string]bool{"f1": true})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeInterval(1*time.Millisecond),
	)
	r.Register(OutputFormat{ID: "f1", Status: FormatUnverified})

	available, unavailable, err := r.ProbeAll(nilContextForFormatRegistryTest())
	if err != nil {
		t.Fatalf("ProbeAll() error = %v", err)
	}
	if available != 1 {
		t.Fatalf("available = %d; want 1", available)
	}
	if unavailable != 0 {
		t.Fatalf("unavailable = %d; want 0", unavailable)
	}
	if prober.callCount() != 1 {
		t.Fatalf("prober called %d times; want 1", prober.callCount())
	}
}

func TestFormatRegistry_ProbeAll_ProbeErrorsAreReturnedSeparately(t *testing.T) {
	prober := newMockProber(map[string]bool{
		"f1": true,
		"f3": false,
	})
	prober.errs = map[string]error{"f2": errors.New("probe failed")}
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeInterval(1*time.Millisecond),
	)
	r.Register(
		OutputFormat{ID: "f1", Status: FormatUnverified},
		OutputFormat{ID: "f2", Status: FormatUnverified},
		OutputFormat{ID: "f3", Status: FormatUnverified},
	)

	available, unavailable, err := r.ProbeAll(context.Background())
	if available != 1 {
		t.Fatalf("available = %d; want 1", available)
	}
	if unavailable != 1 {
		t.Fatalf("unavailable = %d; want 1", unavailable)
	}
	if err == nil {
		t.Fatal("expected aggregated probe error")
	}
	if !errors.Is(err, prober.errs["f2"]) {
		t.Fatalf("expected returned error to include probe failure: %v", err)
	}
	stored, ok := r.Get("f2")
	if !ok {
		t.Fatal("expected f2 to remain registered")
	}
	if stored.Status != FormatUnverified {
		t.Fatalf("f2 status = %v; want FormatUnverified after probe error", stored.Status)
	}
}

func TestFormatRegistry_ProbeTTL(t *testing.T) {
	prober := newMockProber(map[string]bool{"fmt-ttl": true})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeTTL(50*time.Millisecond), // very short TTL for test
	)
	r.Register(OutputFormat{ID: "fmt-ttl", Status: FormatUnverified})

	// First probe
	_, err := r.Probe(context.Background(), "fmt-ttl")
	if err != nil {
		t.Fatalf("first Probe() error: %v", err)
	}
	if prober.callCount() != 1 {
		t.Fatalf("expected 1 probe call, got %d", prober.callCount())
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Second probe should re-probe since TTL expired
	_, err = r.Probe(context.Background(), "fmt-ttl")
	if err != nil {
		t.Fatalf("second Probe() error: %v", err)
	}
	if prober.callCount() != 2 {
		t.Errorf("prober called %d times; want 2 after TTL expiry", prober.callCount())
	}
}

func TestFormatRegistry_IsProbeExpired(t *testing.T) {
	r := NewFormatRegistry(WithProbeTTL(1 * time.Hour))

	fresh := &OutputFormat{
		ID:         "fresh",
		Status:     FormatAvailable,
		VerifiedAt: time.Now(),
	}
	if r.IsProbeExpired(fresh) {
		t.Error("fresh probe result should not be expired")
	}

	stale := &OutputFormat{
		ID:         "stale",
		Status:     FormatAvailable,
		VerifiedAt: time.Now().Add(-2 * time.Hour),
	}
	if !r.IsProbeExpired(stale) {
		t.Error("stale probe result should be expired")
	}

	unverified := &OutputFormat{
		ID:     "unverified",
		Status: FormatUnverified,
	}
	if !r.IsProbeExpired(unverified) {
		t.Error("unverified format with zero VerifiedAt should be considered expired")
	}
}

func TestFormatRegistry_Get_AutoParse(t *testing.T) {
	parser := func(id string) (VoiceAudioProfile, bool) {
		if id == "auto-parsed-format" {
			return VoiceAudioProfile{Format: "mp3", SampleRate: 48000, Bitrate: 192}, true
		}
		return VoiceAudioProfile{}, false
	}
	r := NewFormatRegistry(WithProfileParser(parser))

	// Query an unregistered format — parser should auto-register it
	f, ok := r.Get("auto-parsed-format")
	if !ok {
		t.Fatal("expected auto-parsed format to be found")
	}
	if f.Status != FormatUnverified {
		t.Errorf("status = %v; want FormatUnverified", f.Status)
	}
	if f.Profile.SampleRate != 48000 {
		t.Errorf("SampleRate = %d; want 48000", f.Profile.SampleRate)
	}

	// Subsequent Get should return the cached entry
	f2, ok := r.Get("auto-parsed-format")
	if !ok {
		t.Fatal("expected cached format")
	}
	if f2.Profile.SampleRate != 48000 {
		t.Error("cached format profile mismatch")
	}

	// Unknown format should not be found
	_, ok = r.Get("unknown-format")
	if ok {
		t.Error("expected unknown format not to be found")
	}
}

func TestFormatRegistry_SetProber(t *testing.T) {
	r := NewFormatRegistry()
	r.Register(OutputFormat{ID: "fmt-sp", Status: FormatUnverified})

	// Without prober, Probe should fail
	_, err := r.Probe(context.Background(), "fmt-sp")
	if err == nil {
		t.Fatal("expected error without prober")
	}

	// Set prober dynamically
	prober := newMockProber(map[string]bool{"fmt-sp": true})
	r.SetProber(prober)

	// Now Probe should work
	f, err := r.Probe(context.Background(), "fmt-sp")
	if err != nil {
		t.Fatalf("Probe() error after SetProber: %v", err)
	}
	if f.Status != FormatAvailable {
		t.Errorf("status = %v; want FormatAvailable", f.Status)
	}
}

func TestFormatRegistry_Register_Batch(t *testing.T) {
	r := NewFormatRegistry()
	r.Register(
		OutputFormat{ID: "a", Status: FormatUnverified},
		OutputFormat{ID: "b", Status: FormatAvailable},
		OutputFormat{ID: "c", Status: FormatUnavailable},
	)

	all := r.All()
	if len(all) != 3 {
		t.Errorf("All() = %d; want 3", len(all))
	}

	avail := r.Available()
	if len(avail) != 1 {
		t.Errorf("Available() = %d; want 1", len(avail))
	}
	if avail[0].ID != "b" {
		t.Errorf("Available()[0].ID = %q; want %q", avail[0].ID, "b")
	}
}

func TestFormatRegistry_Declared_TracksExplicitRegistrationsOnly(t *testing.T) {
	parser := func(id string) (VoiceAudioProfile, bool) {
		if id == "auto-parsed" || id == "auto-probed" {
			return VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48}, true
		}
		return VoiceAudioProfile{}, false
	}
	prober := newMockProber(map[string]bool{"auto-probed": true})
	r := NewFormatRegistry(
		WithProfileParser(parser),
		WithProber(prober),
	)
	r.Register(OutputFormat{ID: "declared", Status: FormatUnverified})
	r.RegisterConstant("declared-const", VoiceAudioProfile{Format: "mp3", SampleRate: 24000, Bitrate: 48})

	if _, ok := r.Get("auto-parsed"); !ok {
		t.Fatal("expected auto-parsed format to be registered")
	}
	if _, err := r.Probe(context.Background(), "auto-probed"); err != nil {
		t.Fatalf("Probe() error: %v", err)
	}

	declared := r.Declared()
	if len(declared) != 2 {
		t.Fatalf("Declared() returned %d formats; want 2", len(declared))
	}
	if declared[0].ID != "declared" || declared[1].ID != "declared-const" {
		t.Fatalf("Declared() IDs = [%s %s]; want [declared declared-const]", declared[0].ID, declared[1].ID)
	}

	clone := r.Clone()
	clonedDeclared := clone.Declared()
	if len(clonedDeclared) != len(declared) {
		t.Fatalf("clone.Declared() returned %d formats; want %d", len(clonedDeclared), len(declared))
	}
	for i := range declared {
		if clonedDeclared[i].ID != declared[i].ID {
			t.Fatalf("clone.Declared()[%d].ID = %q; want %q", i, clonedDeclared[i].ID, declared[i].ID)
		}
	}
}

func TestFormatRegistry_ConcurrentAccess(t *testing.T) {
	prober := newMockProber(map[string]bool{
		"c1": true, "c2": false, "c3": true,
	})
	r := NewFormatRegistry(
		WithProber(prober),
		WithProbeTTL(50*time.Millisecond),
	)
	r.Register(
		OutputFormat{ID: "c1", Status: FormatUnverified},
		OutputFormat{ID: "c2", Status: FormatUnverified},
		OutputFormat{ID: "c3", Status: FormatUnverified},
	)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent Get
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Get("c1")
			r.Available()
			r.All()
		}()
	}

	// Concurrent Probe
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _ = r.Probe(ctx, id)
		}(fmt.Sprintf("c%d", (i%3)+1))
	}

	// Concurrent Register
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.RegisterConstant(fmt.Sprintf("dyn-%d", n), VoiceAudioProfile{Format: "mp3"})
		}(i)
	}

	wg.Wait()
	// No race detector panics = success
}
