package service

type ID string
type BoundaryType string

const (
	BoundaryHTTP  BoundaryType = "http"
	BoundaryGRPC  BoundaryType = "grpc"
	BoundaryKafka BoundaryType = "kafka"
)

type Manifest struct {
	ID          ID             `json:"id"`
	Name        string         `json:"name"`
	RepositoryID string        `json:"repository_id"`
	RootPath    string         `json:"root_path"`
	Entrypoints []string       `json:"entrypoints"`
	Boundaries  []BoundaryType `json:"boundaries"`
}
