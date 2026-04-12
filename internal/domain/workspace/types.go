package workspace

import "time"

type ID string
type Path string

type Manifest struct {
	ID            ID        `json:"id"`
	RootPath      Path      `json:"root_path"`
	RepositoryIDs []string  `json:"repository_ids"`
	Languages     []string  `json:"languages"`
	Warnings      []string  `json:"warnings"`
	ScannedAt     time.Time `json:"scanned_at"`
}
