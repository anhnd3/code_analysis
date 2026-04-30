package artifactstore

import (
	"analysis-module/internal/domain/artifact"
)

type Store interface {
	SaveJSON(workspaceID, snapshotID, fileName string, artifactType artifact.Type, payload any) (artifact.Ref, error)
	SaveText(workspaceID, snapshotID, fileName string, artifactType artifact.Type, body string) (artifact.Ref, error)
}
