package daneel

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// NewSessionID generates a random UUID v4 session identifier.
func NewSessionID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// DeterministicSessionID produces a stable session ID from a set of keys.
// Used by the Bridge to map (platform, user, channel) to a consistent
// session, so the same user in the same channel always resumes their
// conversation.
//
//	id := DeterministicSessionID("telegram", "user123", "group456")
func DeterministicSessionID(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte(":"))
		}
		h.Write([]byte(p))
	}
	sum := h.Sum(nil)
	// Format as UUID-like string from first 16 bytes of hash.
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
