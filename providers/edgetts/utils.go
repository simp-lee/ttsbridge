package edgetts

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// generateRequestID generates unique request ID
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// generateConnectionID generates UUID-style connection ID (no dashes)
func generateConnectionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant bits
	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getTimestamp returns JavaScript-style timestamp
func getTimestamp() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05") + " GMT+0000 (Coordinated Universal Time)"
}

// generateMUID creates an uppercase MUID cookie identifier
func generateMUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strings.ToUpper(fmt.Sprintf("%x", time.Now().UnixNano()))
	}
	return strings.ToUpper(hex.EncodeToString(b))
}

func makeMUIDCookie() string {
	return fmt.Sprintf("muid=%s;", generateMUID())
}
