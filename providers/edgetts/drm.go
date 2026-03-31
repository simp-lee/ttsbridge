package edgetts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	winEpoch             = 11644473600 // Windows epoch: 1601-01-01 00:00:00 UTC
	sToNs                = 1e9         // Seconds to nanoseconds
	tokenValiditySeconds = 300         // 5 minutes
)

var drmTimeNow = time.Now

// generateSecMsGec generates the Sec-MS-GEC token value using the provider's
// per-instance clock skew state.
// Reference: https://github.com/rany2/edge-tts/blob/master/src/edge_tts/drm.py
func (p *Provider) generateSecMsGec() string {
	p.clockSkewMu.RLock()
	skew := p.clockSkewSecs
	p.clockSkewMu.RUnlock()

	// Get current timestamp with clock skew correction
	now := drmTimeNow().UTC()
	ticks := float64(now.Unix()) + float64(now.Nanosecond())/sToNs + skew

	// Convert to Windows file time epoch and round down to nearest 5 minutes
	ticks += winEpoch

	// Round down to nearest 5 minutes
	ticks -= math.Mod(ticks, float64(tokenValiditySeconds))

	// Convert to 100-nanosecond intervals
	ticks *= sToNs / 100

	// Compute SHA256 hash
	strToHash := fmt.Sprintf("%.0f%s", ticks, p.clientToken)
	hash := sha256.Sum256([]byte(strToHash))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// adjustClockSkew adjusts the provider's clock skew based on server time.
// Called when a 403 error is received.
// Note: This uses additive adjustment like the Python reference implementation.
func (p *Provider) adjustClockSkew(serverDate string) error {
	serverTime, err := time.Parse(time.RFC1123, serverDate)
	if err != nil {
		return fmt.Errorf("failed to parse server date: %w", err)
	}

	p.clockSkewMu.Lock()
	defer p.clockSkewMu.Unlock()

	// Calculate skew delta using existing skew-adjusted client time.
	// Match Python's dt.now(tz.utc).timestamp() + clock_skew_seconds
	now := drmTimeNow().UTC()
	clientTime := float64(now.Unix()) + float64(now.Nanosecond())/sToNs + p.clockSkewSecs
	serverUnix := float64(serverTime.Unix()) + float64(serverTime.Nanosecond())/sToNs
	p.clockSkewSecs += serverUnix - clientTime

	return nil
}
