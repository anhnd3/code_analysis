package snapshot_manage

import (
	"fmt"
	"time"

	"analysis-module/pkg/ids"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) NewWorkspaceID(root string) string {
	return ids.Stable("ws", root)
}

func (Service) NewSnapshotID(workspaceID string) string {
	return fmt.Sprintf("%s_%s", workspaceID, time.Now().UTC().Format("20060102T150405Z"))
}
