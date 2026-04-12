package packet_build

import (
	apperrors "analysis-module/internal/app/errors"
	"analysis-module/internal/domain/packet"
)

type Service struct{}

func New() Service {
	return Service{}
}

func (Service) Build(target, snapshotID string) (packet.Packet, error) {
	return packet.Packet{}, apperrors.NotImplemented("packet building is deferred in this pass")
}
