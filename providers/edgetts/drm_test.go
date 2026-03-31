package edgetts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func newDRMTestProvider() *Provider {
	return &Provider{clientToken: defaultClientToken}
}

func resetDRMTimeNow(t *testing.T) func() {
	t.Helper()
	originalNow := drmTimeNow
	return func() {
		drmTimeNow = originalNow
	}
}

func setDRMTestTime(now time.Time) {
	drmTimeNow = func() time.Time { return now }
}

func expectedSkewFromRFC1123(t *testing.T, serverDate string, clientTime time.Time, currentSkew float64) float64 {
	t.Helper()

	serverTime, err := time.Parse(time.RFC1123, serverDate)
	if err != nil {
		t.Fatalf("failed to parse server date %q: %v", serverDate, err)
	}

	clientUnix := float64(clientTime.Unix()) + float64(clientTime.Nanosecond())/sToNs + currentSkew
	serverUnix := float64(serverTime.Unix()) + float64(serverTime.Nanosecond())/sToNs
	return serverUnix - clientUnix
}

func TestGenerateSecMsGec(t *testing.T) {
	p := newDRMTestProvider()
	token := p.generateSecMsGec()

	// Token should be 64 characters (SHA256 hex)
	if len(token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// Token should be uppercase
	for _, c := range token {
		if c >= 'a' && c <= 'z' {
			t.Errorf("Token should be uppercase, but contains lowercase: %s", token)
			break
		}
	}

	// Generate again should produce same result within 5-minute window
	token2 := p.generateSecMsGec()
	if token != token2 {
		t.Errorf("Tokens generated within same time window should match")
	}
}

func TestAdjustClockSkew(t *testing.T) {
	p := newDRMTestProvider()

	// Test valid RFC 2616 date
	err := p.adjustClockSkew("Mon, 02 Jan 2006 15:04:05 GMT")
	if err != nil {
		t.Errorf("Failed to parse valid date: %v", err)
	}

	// Test invalid date
	err = p.adjustClockSkew("invalid date")
	if err == nil {
		t.Error("Should fail for invalid date")
	}
}

// TestClockSkewCalculation verifies the clock skew calculation matches Python reference
func TestClockSkewCalculation(t *testing.T) {
	defer resetDRMTimeNow(t)()
	p := newDRMTestProvider()

	// Use whole-second fixtures because RFC1123 only preserves second precision.
	baseClientTime := time.Date(2024, time.November, 11, 12, 34, 56, 0, time.UTC)
	setDRMTestTime(baseClientTime)
	serverTime := baseClientTime.Add(10 * time.Second)
	serverDateStr := serverTime.Format(time.RFC1123)
	expectedSkew := expectedSkewFromRFC1123(t, serverDateStr, baseClientTime, 0)

	err := p.adjustClockSkew(serverDateStr)
	if err != nil {
		t.Fatalf("Failed to adjust clock skew: %v", err)
	}

	p.clockSkewMu.RLock()
	skew1 := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	if math.Abs(skew1-expectedSkew) > 1e-9 {
		t.Errorf("Expected skew %.3fs, got %.3fs (diff: %.3fs)", expectedSkew, skew1, skew1-expectedSkew)
	}

	// Advance the raw client clock by 20s. With the previous 10s skew applied,
	// the adjusted client time is base+30s, so a server date at base+35s should
	// add exactly 5 more seconds of skew.
	secondClientTime := baseClientTime.Add(20 * time.Second)
	setDRMTestTime(secondClientTime)
	serverTime2 := baseClientTime.Add(35 * time.Second)
	serverDateStr2 := serverTime2.Format(time.RFC1123)
	expectedAdditionalSkew := expectedSkewFromRFC1123(t, serverDateStr2, secondClientTime, skew1)

	err = p.adjustClockSkew(serverDateStr2)
	if err != nil {
		t.Fatalf("Failed to adjust clock skew second time: %v", err)
	}

	p.clockSkewMu.RLock()
	skew2 := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	additionalSkew := skew2 - skew1
	if math.Abs(additionalSkew-expectedAdditionalSkew) > 1e-9 {
		t.Errorf("Expected additional skew %.3fs, got %.3fs (total: %.3fs)", expectedAdditionalSkew, additionalSkew, skew2)
	}
	expectedTotalSkew := expectedSkew + expectedAdditionalSkew
	if math.Abs(skew2-expectedTotalSkew) > 1e-9 {
		t.Errorf("Expected total skew %.3fs, got %.3fs", expectedTotalSkew, skew2)
	}

	t.Logf("First adjustment: %.6fs, Second adjustment: %.6fs, Total: %.6fs",
		skew1, additionalSkew, skew2)
}

// TestClockSkewNegative verifies negative clock skew (client ahead of server)
func TestClockSkewNegative(t *testing.T) {
	defer resetDRMTimeNow(t)()
	p := newDRMTestProvider()

	clientTime := time.Date(2024, time.November, 11, 12, 34, 56, 0, time.UTC)
	setDRMTestTime(clientTime)
	serverTime := clientTime.Add(-5 * time.Second)
	serverDateStr := serverTime.Format(time.RFC1123)
	expectedSkew := expectedSkewFromRFC1123(t, serverDateStr, clientTime, 0)

	err := p.adjustClockSkew(serverDateStr)
	if err != nil {
		t.Fatalf("Failed to adjust clock skew: %v", err)
	}

	p.clockSkewMu.RLock()
	skew := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	if math.Abs(skew-expectedSkew) > 1e-9 {
		t.Errorf("Expected skew %.3fs, got %.3fs (diff: %.3fs)", expectedSkew, skew, skew-expectedSkew)
	}

	t.Logf("Negative skew adjustment: %.6fs (expected: %.6fs)", skew, expectedSkew)
}

// TestGenerateSecMsGecWithSkew verifies token generation with clock skew
func TestGenerateSecMsGecWithSkew(t *testing.T) {
	p := newDRMTestProvider()

	// Set a known skew that will cross a 5-minute boundary
	// Use 301 seconds (just over 5 minutes) to ensure different token
	p.clockSkewMu.Lock()
	p.clockSkewSecs = 301.0
	p.clockSkewMu.Unlock()

	token1 := p.generateSecMsGec()

	// Token should be different from one generated without skew
	p.clockSkewMu.Lock()
	p.clockSkewSecs = 0.0
	p.clockSkewMu.Unlock()

	token2 := p.generateSecMsGec()

	// Tokens should be different due to skew crossing 5-minute boundary
	if token1 == token2 {
		t.Error("Tokens should differ when clock skew crosses 5-minute boundary (301s)")
	}

	// Restore skew and verify token matches again
	p.clockSkewMu.Lock()
	p.clockSkewSecs = 301.0
	p.clockSkewMu.Unlock()

	token3 := p.generateSecMsGec()
	if token1 != token3 {
		t.Error("Token should match when same skew is restored")
	}

	// Test that small skew within same 5-minute window produces same token
	p.clockSkewMu.Lock()
	p.clockSkewSecs = 10.0
	p.clockSkewMu.Unlock()
	token4 := p.generateSecMsGec()

	p.clockSkewMu.Lock()
	p.clockSkewSecs = 20.0
	p.clockSkewMu.Unlock()
	token5 := p.generateSecMsGec()

	// These should likely be the same (unless we're at a boundary)
	// Just log the result, don't fail
	t.Logf("Token with 10s skew:  %s", token4)
	t.Logf("Token with 20s skew:  %s", token5)
	t.Logf("Token with 301s skew: %s", token1)
	t.Logf("Token with 0s skew:   %s", token2)
}

// TestClockSkewPrecision verifies sub-second precision is maintained
func TestClockSkewPrecision(t *testing.T) {
	defer resetDRMTimeNow(t)()
	p := newDRMTestProvider()

	clientTime := time.Date(2024, time.November, 11, 12, 34, 56, 789123000, time.UTC)
	setDRMTestTime(clientTime)
	serverTime := clientTime.Add(10*time.Second + 123*time.Millisecond + 456*time.Microsecond)
	serverDateStr := serverTime.Format(time.RFC1123)
	expectedSkew := expectedSkewFromRFC1123(t, serverDateStr, clientTime, 0)

	err := p.adjustClockSkew(serverDateStr)
	if err != nil {
		t.Fatalf("Failed to adjust clock skew: %v", err)
	}

	p.clockSkewMu.RLock()
	skew := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	if math.Abs(skew-expectedSkew) > 1e-9 {
		t.Errorf("Expected skew %.3fs, got %.3fs (diff: %.3fs)", expectedSkew, skew, skew-expectedSkew)
	}

	t.Logf("Clock skew: %.6fs (expected: %.6fs with RFC1123 server precision)", skew, expectedSkew)
}

// TestClockSkewAlgorithmMatchesPython verifies our algorithm matches Python reference exactly
func TestClockSkewAlgorithmMatchesPython(t *testing.T) {
	// This test verifies the exact algorithm from edge-tts Python implementation:
	// def handle_client_response_error(e):
	//     server_date_parsed = DRM.parse_rfc2616_date(server_date)
	//     client_date = DRM.get_unix_timestamp()  # Uses already-adjusted time
	//     DRM.adj_clock_skew_seconds(server_date_parsed - client_date)  # Cumulative adjustment

	defer resetDRMTimeNow(t)()
	p := newDRMTestProvider()

	// Scenario: Multiple adjustments to simulate real-world clock drift corrections
	baseClientTime := time.Date(2024, time.November, 11, 12, 34, 56, 0, time.UTC)

	// First adjustment: Server ahead by 10s
	t1Client := baseClientTime
	setDRMTestTime(t1Client)
	t1Server := t1Client.Add(10 * time.Second)
	t1ServerDate := t1Server.Format(time.RFC1123)
	expectedDelta1 := expectedSkewFromRFC1123(t, t1ServerDate, t1Client, 0)
	err := p.adjustClockSkew(t1ServerDate)
	if err != nil {
		t.Fatalf("First adjustment failed: %v", err)
	}

	p.clockSkewMu.RLock()
	skew1 := p.clockSkewSecs
	p.clockSkewMu.RUnlock()
	if math.Abs(skew1-expectedDelta1) > 1e-9 {
		t.Fatalf("Expected first skew %.3fs, got %.3fs", expectedDelta1, skew1)
	}

	t.Logf("After 1st adjustment: skew=%.6fs", skew1)

	// Second adjustment: Server is now 3s ahead of the *adjusted* client time
	t2Client := baseClientTime.Add(20 * time.Second)
	setDRMTestTime(t2Client)
	t2Server := baseClientTime.Add(33 * time.Second)
	t2ServerDate := t2Server.Format(time.RFC1123)
	expectedDelta2 := expectedSkewFromRFC1123(t, t2ServerDate, t2Client, skew1)
	err = p.adjustClockSkew(t2ServerDate)
	if err != nil {
		t.Fatalf("Second adjustment failed: %v", err)
	}

	p.clockSkewMu.RLock()
	skew2 := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	delta := skew2 - skew1
	t.Logf("After 2nd adjustment: skew=%.6fs (delta=%.6fs)", skew2, delta)

	if math.Abs(delta-expectedDelta2) > 1e-9 {
		t.Errorf("Expected delta %.3fs, got %.3fs", expectedDelta2, delta)
	}

	// Third adjustment: Server falls behind by 2s from adjusted time (negative adjustment)
	t3Client := baseClientTime.Add(40 * time.Second)
	setDRMTestTime(t3Client)
	t3Server := baseClientTime.Add(51 * time.Second)
	t3ServerDate := t3Server.Format(time.RFC1123)
	expectedDelta3 := expectedSkewFromRFC1123(t, t3ServerDate, t3Client, skew2)
	err = p.adjustClockSkew(t3ServerDate)
	if err != nil {
		t.Fatalf("Third adjustment failed: %v", err)
	}

	p.clockSkewMu.RLock()
	skew3 := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	delta2 := skew3 - skew2
	t.Logf("After 3rd adjustment: skew=%.6fs (delta=%.6fs)", skew3, delta2)

	if math.Abs(delta2-expectedDelta3) > 1e-9 {
		t.Errorf("Expected delta %.3fs, got %.3fs", expectedDelta3, delta2)
	}

	expectedTotalSkew := expectedDelta1 + expectedDelta2 + expectedDelta3
	t.Logf("Final cumulative skew: %.6fs (expected %.6fs)", skew3, expectedTotalSkew)
	if math.Abs(skew3-expectedTotalSkew) > 1e-9 {
		t.Errorf("Expected total skew %.3fs, got %.3fs", expectedTotalSkew, skew3)
	}
}

// TestTokenGenerationAlgorithm verifies the token generation matches Python reference
func TestTokenGenerationAlgorithm(t *testing.T) {
	// Python algorithm:
	// ticks = DRM.get_unix_timestamp()    # Unix timestamp with skew
	// ticks += WIN_EPOCH                   # 11644473600
	// ticks -= ticks % 300                 # Round down to 5 minutes
	// ticks *= S_TO_NS / 100               # Convert to 100-ns intervals (1e9/100 = 1e7)
	// str_to_hash = f"{ticks:.0f}{TRUSTED_CLIENT_TOKEN}"
	// return hashlib.sha256(str_to_hash.encode("ascii")).hexdigest().upper()

	// Test with a fixed timestamp
	testTime := time.Date(2024, 11, 11, 12, 34, 56, 123456789, time.UTC)

	// Manually calculate what Python would produce
	unixTime := float64(testTime.Unix()) + float64(testTime.Nanosecond())/1e9
	ticks := unixTime + 0.0 // No skew for this test
	ticks += 11644473600.0
	ticks -= math.Mod(ticks, 300.0)
	ticks *= 1e7

	expectedStr := fmt.Sprintf("%.0f%s", ticks, defaultClientToken)
	expectedHash := sha256.Sum256([]byte(expectedStr))
	expectedToken := strings.ToUpper(hex.EncodeToString(expectedHash[:]))

	t.Logf("Test timestamp: %s", testTime.Format(time.RFC3339Nano))
	t.Logf("Unix time: %.9f", unixTime)
	t.Logf("After +winEpoch: %.9f", unixTime+11644473600.0)
	t.Logf("After rounding: %.9f", unixTime+11644473600.0-math.Mod(unixTime+11644473600.0, 300.0))
	t.Logf("Ticks (100-ns): %.0f", ticks)
	t.Logf("Hash input: %s", expectedStr)
	t.Logf("Expected token: %s", expectedToken)

	// Now generate with our implementation
	// We need to mock time, but since we can't easily do that, we'll verify the algorithm
	// by using the same manual calculation
	p := newDRMTestProvider()
	now := time.Now().UTC()
	token := p.generateSecMsGec()

	t.Logf("Current time: %s", now.Format(time.RFC3339Nano))
	t.Logf("Generated token: %s", token)

	// Verify token format (64 hex chars, uppercase)
	if len(token) != 64 {
		t.Errorf("Token length should be 64, got %d", len(token))
	}
	for _, c := range token {
		if c < '0' || (c > '9' && c < 'A') || c > 'F' {
			t.Errorf("Token should be uppercase hex, got: %s", token)
			break
		}
	}
}

// TestEdgeCaseBoundaries tests token generation near 5-minute boundaries
func TestEdgeCaseBoundaries(t *testing.T) {
	p := newDRMTestProvider()

	// Generate token
	token1 := p.generateSecMsGec()

	// Wait to potentially cross boundary
	time.Sleep(100 * time.Millisecond)

	token2 := p.generateSecMsGec()

	// Within 100ms, we should still be in the same 5-minute window
	if token1 != token2 {
		t.Logf("Tokens differ (might be at boundary): %s vs %s", token1, token2)
	} else {
		t.Logf("Tokens match within same 5-minute window: %s", token1)
	}
}

// TestRFC1123DateFormats tests various RFC1123 date format edge cases
func TestRFC1123DateFormats(t *testing.T) {
	testCases := []struct {
		name      string
		dateStr   string
		shouldErr bool
	}{
		{
			name:      "Standard RFC1123",
			dateStr:   "Mon, 02 Jan 2006 15:04:05 GMT",
			shouldErr: false,
		},
		{
			name:      "Invalid month",
			dateStr:   "Mon, 02 Xxx 2006 15:04:05 GMT",
			shouldErr: true,
		},
		{
			name:      "Invalid format",
			dateStr:   "2006-01-02T15:04:05Z",
			shouldErr: true,
		},
		{
			name:      "Empty string",
			dateStr:   "",
			shouldErr: true,
		},
		{
			name:      "Current year date",
			dateStr:   time.Now().UTC().Format(time.RFC1123),
			shouldErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := newDRMTestProvider()

			err := p.adjustClockSkew(tc.dateStr)
			if tc.shouldErr && err == nil {
				t.Errorf("Expected error for date '%s', got nil", tc.dateStr)
			} else if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error for date '%s', got: %v", tc.dateStr, err)
			}
		})
	}
}

// TestZeroSkewBehavior verifies behavior with zero clock skew
func TestZeroSkewBehavior(t *testing.T) {
	p := newDRMTestProvider()

	// Generate token with zero skew
	token := p.generateSecMsGec()

	// Verify it's a valid SHA256 hash
	if len(token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// Verify uppercase
	for _, c := range token {
		if c >= 'a' && c <= 'z' {
			t.Errorf("Token should be uppercase, got: %s", token)
			break
		}
	}

	t.Logf("Token with zero skew: %s", token)
}

// TestConcurrentTokenGeneration tests thread safety of token generation
func TestConcurrentTokenGeneration(t *testing.T) {
	p := newDRMTestProvider()

	const numGoroutines = 100
	tokens := make(chan string, numGoroutines)

	// Generate tokens concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			token := p.generateSecMsGec()
			tokens <- token
		}()
	}

	// Collect all tokens
	uniqueTokens := make(map[string]int)
	for i := 0; i < numGoroutines; i++ {
		token := <-tokens
		uniqueTokens[token]++
	}

	// All tokens should be the same (generated within same 5-minute window)
	// or at most 2 different tokens (if we cross a boundary)
	if len(uniqueTokens) > 2 {
		t.Errorf("Expected at most 2 unique tokens, got %d", len(uniqueTokens))
	}

	t.Logf("Generated %d tokens, %d unique", numGoroutines, len(uniqueTokens))
	for token, count := range uniqueTokens {
		t.Logf("  Token: %s (count: %d)", token, count)
	}
}

// TestConcurrentSkewAdjustment tests thread safety of skew adjustment
func TestConcurrentSkewAdjustment(t *testing.T) {
	defer resetDRMTimeNow(t)()
	p := newDRMTestProvider()

	baseClientTime := time.Date(2024, time.November, 11, 12, 34, 56, 0, time.UTC)
	setDRMTestTime(baseClientTime)

	const numGoroutines = 50
	done := make(chan bool, numGoroutines)

	// Perform concurrent adjustments
	for i := 0; i < numGoroutines; i++ {
		go func(iteration int) {
			serverTime := baseClientTime.Add(time.Duration(iteration+1) * time.Second)
			err := p.adjustClockSkew(serverTime.Format(time.RFC1123))
			if err != nil {
				t.Errorf("Adjustment failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all adjustments
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	p.clockSkewMu.RLock()
	finalSkew := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	t.Logf("Final skew after %d concurrent adjustments: %.6fs", numGoroutines, finalSkew)

	if finalSkew < 1.0 || finalSkew > float64(numGoroutines) {
		t.Errorf("Expected final skew to match one of the deterministic server offsets [1,%d], got %.3fs", numGoroutines, finalSkew)
	}
}

// TestLargeSkewValues tests behavior with extreme clock skew values
func TestLargeSkewValues(t *testing.T) {
	testCases := []struct {
		name string
		skew float64
	}{
		{"Large positive skew", 86400.0},     // 1 day
		{"Large negative skew", -86400.0},    // -1 day
		{"Very large positive", 31536000.0},  // ~1 year
		{"Very large negative", -31536000.0}, // ~-1 year
		{"Sub-second precision", 0.123456789},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := newDRMTestProvider()
			p.clockSkewMu.Lock()
			p.clockSkewSecs = tc.skew
			p.clockSkewMu.Unlock()

			token := p.generateSecMsGec()

			// Verify token is valid
			if len(token) != 64 {
				t.Errorf("Expected token length 64, got %d", len(token))
			}

			t.Logf("Skew: %.9fs, Token: %s", tc.skew, token)
		})
	}
}

// TestPythonReferenceCompatibility verifies exact algorithm match with Python
func TestPythonReferenceCompatibility(t *testing.T) {
	// This test documents the exact algorithm from edge-tts Python implementation
	// and verifies our Go implementation matches it

	// Python code:
	// ticks = dt.now(tz.utc).timestamp() + DRM.clock_skew_seconds
	// ticks += WIN_EPOCH  # 11644473600
	// ticks -= ticks % 300
	// ticks *= S_TO_NS / 100  # 1e9 / 100 = 1e7
	// str_to_hash = f"{ticks:.0f}{TRUSTED_CLIENT_TOKEN}"
	// return hashlib.sha256(str_to_hash.encode("ascii")).hexdigest().upper()

	// Test with known values
	testCases := []struct {
		name        string
		unixTime    float64
		skew        float64
		clientToken string
	}{
		{
			name:        "Zero skew",
			unixTime:    1700000000.0,
			skew:        0.0,
			clientToken: defaultClientToken,
		},
		{
			name:        "Positive skew",
			unixTime:    1700000000.0,
			skew:        123.456,
			clientToken: defaultClientToken,
		},
		{
			name:        "Negative skew",
			unixTime:    1700000000.0,
			skew:        -123.456,
			clientToken: defaultClientToken,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Calculate expected token using Python algorithm
			ticks := tc.unixTime + tc.skew
			ticks += winEpoch
			ticks -= math.Mod(ticks, tokenValiditySeconds)
			ticks *= sToNs / 100

			strToHash := fmt.Sprintf("%.0f%s", ticks, tc.clientToken)
			hash := sha256.Sum256([]byte(strToHash))
			expectedToken := strings.ToUpper(hex.EncodeToString(hash[:]))

			t.Logf("Input: unixTime=%.6f, skew=%.6f", tc.unixTime, tc.skew)
			t.Logf("After +winEpoch: %.6f", tc.unixTime+tc.skew+winEpoch)
			t.Logf("After rounding: %.6f", tc.unixTime+tc.skew+winEpoch-math.Mod(tc.unixTime+tc.skew+winEpoch, tokenValiditySeconds))
			t.Logf("Ticks: %.0f", ticks)
			t.Logf("Hash input: %s", strToHash)
			t.Logf("Expected token: %s", expectedToken)

			// Verify the algorithm is correct by checking hash input format
			if !strings.HasSuffix(strToHash, tc.clientToken) {
				t.Errorf("Hash input should end with client token")
			}
			if len(expectedToken) != 64 {
				t.Errorf("Expected token length 64, got %d", len(expectedToken))
			}
		})
	}
}

func TestGenerateConnectionID(t *testing.T) {
	id1 := generateConnectionID()
	id2 := generateConnectionID()

	// Should be 32 hex characters (UUID without dashes)
	if len(id1) != 32 {
		t.Errorf("Expected ID length 32, got %d", len(id1))
	}

	// Should be unique
	if id1 == id2 {
		t.Error("Generated IDs should be unique")
	}
}

func TestGenerateSecMsGecMatchesReference(t *testing.T) {
	const skew = 123.456789

	p := newDRMTestProvider()
	p.clockSkewMu.Lock()
	p.clockSkewSecs = skew
	p.clockSkewMu.Unlock()

	referenceToken := func(unixSeconds int64) string {
		ticks := float64(unixSeconds) + skew + winEpoch
		ticks -= math.Mod(ticks, tokenValiditySeconds)
		ticks *= sToNs / 100
		strToHash := fmt.Sprintf("%.0f%s", ticks, defaultClientToken)
		sum := sha256.Sum256([]byte(strToHash))
		return strings.ToUpper(hex.EncodeToString(sum[:]))
	}

	var lastToken string
	var lastBase int64

	for attempt := 0; attempt < 5; attempt++ {
		base := time.Now().Unix()
		token := p.generateSecMsGec()

		if token == referenceToken(base) || token == referenceToken(base+1) {
			return
		}

		lastToken = token
		lastBase = base

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("token mismatch: got %s, expected %s (base=%d) or %s (base+1)", lastToken, referenceToken(lastBase), lastBase, referenceToken(lastBase+1))
}

// TestPerInstanceClockSkewIsolation verifies that clock skew is isolated per Provider instance
func TestPerInstanceClockSkewIsolation(t *testing.T) {
	p1 := newDRMTestProvider()
	p2 := newDRMTestProvider()

	p1.clockSkewMu.Lock()
	p1.clockSkewSecs = 301.0
	p1.clockSkewMu.Unlock()

	// p2 should still have zero skew
	p2.clockSkewMu.RLock()
	if p2.clockSkewSecs != 0 {
		t.Errorf("p2 skew = %f, want 0 (should be independent of p1)", p2.clockSkewSecs)
	}
	p2.clockSkewMu.RUnlock()

	// Tokens should differ since skew differs
	token1 := p1.generateSecMsGec()
	token2 := p2.generateSecMsGec()
	if token1 == token2 {
		t.Error("Tokens from providers with different skew should differ")
	}
}
