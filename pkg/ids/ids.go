package ids

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// Stable returns a short deterministic identifier derived from the provided parts.
func Stable(prefix string, parts ...string) string {
	return prefix + "_" + StableSuffix(parts...)
}

// StableSuffix returns the deterministic hash fragment used by Stable.
func StableSuffix(parts ...string) string {
	return stableDigest(parts...)
}

// Slug normalizes a human-readable identifier into a filesystem-safe ASCII slug.
func Slug(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if builder.Len() == 0 || lastUnderscore {
				continue
			}
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	slug := strings.Trim(builder.String(), "_")
	if slug == "" {
		return "workspace"
	}
	return slug
}

func stableDigest(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
