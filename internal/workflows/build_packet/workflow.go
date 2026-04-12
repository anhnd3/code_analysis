package build_packet

import (
	apperrors "analysis-module/internal/app/errors"
	"analysis-module/internal/domain/packet"
)

type Request struct {
	SnapshotID string `json:"snapshot_id"`
	Target     string `json:"target"`
}

type Result struct {
	Packet packet.Packet `json:"packet"`
}

func Run(Request) (Result, error) {
	return Result{}, apperrors.NotImplemented("build-packet workflow is deferred in this pass")
}
