package snapshot_manage

import (
	"fmt"
	"path/filepath"
	"time"

	"analysis-module/pkg/ids"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) NewWorkspaceID(root string) string {
	cleanRoot := filepath.Clean(root)
	slug := ids.Slug(filepath.Base(cleanRoot))
	return fmt.Sprintf("ws_%s_%s", slug, ids.StableSuffix(cleanRoot))
}

func (Service) NewSnapshotID(workspaceID string) string {
	return fmt.Sprintf("%s_%s", workspaceID, time.Now().UTC().Format("20060102T150405Z"))
}
