package ids

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// Stable returns a short deterministic identifier derived from the provided parts.
func Stable(prefix string, parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(h.Sum(nil))[:12]
}
