package artifactstore

import (
	"analysis-module/internal/domain/artifact"
	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/quality"
)

type Store interface {
	SaveJSON(workspaceID, snapshotID, fileName string, artifactType artifact.Type, payload any) (artifact.Ref, error)
	SaveGraph(workspaceID, snapshotID string, nodes []graph.Node, edges []graph.Edge) ([]artifact.Ref, error)
	SaveQualityReport(workspaceID, snapshotID string, report quality.AnalysisQualityReport) (artifact.Ref, error)
	SaveText(workspaceID, snapshotID, fileName string, artifactType artifact.Type, body string) (artifact.Ref, error)
}
