package indexer

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// SnapshotService generates workspace and snapshot identifiers.
type SnapshotService struct{}

func NewSnapshotService() SnapshotService {
	return SnapshotService{}
}

func (SnapshotService) NewWorkspaceID(root string) string {
	cleanRoot := filepath.Clean(root)
	slug := slugify(filepath.Base(cleanRoot))
	return fmt.Sprintf("ws_%s_%s", slug, stableSuffix(cleanRoot))
}

func (SnapshotService) NewSnapshotID(workspaceID string) string {
	return fmt.Sprintf("%s_%s", workspaceID, time.Now().UTC().Format("20060102T150405Z"))
}

// ─── ID generation helpers ─────────────────────────────

func StableID(prefix string, parts ...string) string {
	return prefix + "_" + stableSuffix(parts...)
}

func stableSuffix(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func slugify(raw string) string {
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
