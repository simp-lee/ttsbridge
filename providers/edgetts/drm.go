package edgetts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// Windows epoch: 1601-01-01 00:00:00 UTC
	winEpoch = 11644473600

	// Seconds to nanoseconds conversion
	sToNs = 1e9

	// Token validity window: 5 minutes
	tokenValiditySeconds = 300
)

var (
	// clockSkewSeconds stores the time difference between client and server
	clockSkewSeconds float64
	clockSkewMutex   sync.RWMutex
)

// GenerateSecMsGec generates the Sec-MS-GEC token value.
//
// This function generates a token based on the current time in Windows file time format
// adjusted for clock skew, and rounded down to the nearest 5 minutes. The token is then
// hashed using SHA256 and returned as an uppercased hex digest.
//
// Algorithm:
// 1. Get current Unix timestamp with clock skew correction
// 2. Convert to Windows file time (epoch: 1601-01-01)
// 3. Round down to nearest 5 minutes
// 4. Convert to 100-nanosecond intervals
// 5. Concatenate with trusted client token
// 6. Compute SHA256 hash and return uppercase hex
//
// Reference: https://github.com/rany2/edge-tts/issues/290#issuecomment-2464956570
func GenerateSecMsGec(clientToken string) string {
	clockSkewMutex.RLock()
	skew := clockSkewSeconds
	clockSkewMutex.RUnlock()

	// Get current timestamp with clock skew correction
	ticks := float64(time.Now().Unix()) + skew

	// Convert to Windows file time epoch
	ticks += winEpoch

	// Round down to nearest 5 minutes
	ticks -= float64(int64(ticks) % tokenValiditySeconds)

	// Convert to 100-nanosecond intervals
	ticks *= sToNs / 100

	// Create string to hash
	strToHash := fmt.Sprintf("%.0f%s", ticks, clientToken)

	// Compute SHA256 and return uppercase hex
	hash := sha256.Sum256([]byte(strToHash))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// AdjustClockSkew adjusts the clock skew based on server time.
//
// This is called when a 403 error is received, indicating that the client
// time is not synchronized with the server time.
//
// Parameters:
//   - serverDate: RFC 2616 formatted date string from server response header
//
// Example server date: "Mon, 02 Jan 2006 15:04:05 GMT"
func AdjustClockSkew(serverDate string) error {
	// Parse RFC 2616 date format
	layout := "Mon, 02 Jan 2006 15:04:05 MST"
	serverTime, err := time.Parse(layout, serverDate)
	if err != nil {
		return fmt.Errorf("failed to parse server date '%s': %w", serverDate, err)
	}

	// Calculate skew
	clientTime := time.Now()
	skew := serverTime.Sub(clientTime).Seconds()

	// Update global clock skew (直接覆盖而不是累加，避免偏差越来越大)
	clockSkewMutex.Lock()
	clockSkewSeconds = skew
	clockSkewMutex.Unlock()

	return nil
}

// GetClockSkew returns the current clock skew in seconds
func GetClockSkew() float64 {
	clockSkewMutex.RLock()
	defer clockSkewMutex.RUnlock()
	return clockSkewSeconds
}

// ResetClockSkew resets the clock skew to zero (useful for testing)
func ResetClockSkew() {
	clockSkewMutex.Lock()
	clockSkewSeconds = 0
	clockSkewMutex.Unlock()
}
