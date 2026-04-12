package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	ArtifactRoot string
	SQLitePath   string
	HTTPAddr     string
	ProgressMode string
}

func Default() Config {
	artifactRoot := filepath.Clean("artifacts")
	sqlitePath := ""
	if value := os.Getenv("ANALYSIS_ARTIFACT_ROOT"); value != "" {
		artifactRoot = filepath.Clean(value)
	}
	if value := os.Getenv("ANALYSIS_SQLITE_PATH"); value != "" {
		sqlitePath = filepath.Clean(value)
	}
	addr := ":8080"
	if value := os.Getenv("ANALYSIS_HTTP_ADDR"); value != "" {
		addr = value
	}
	progressMode := "auto"
	if value := os.Getenv("ANALYSIS_PROGRESS_MODE"); value != "" {
		progressMode = value
	}
	return Config{
		ArtifactRoot: artifactRoot,
		SQLitePath:   sqlitePath,
		HTTPAddr:     addr,
		ProgressMode: progressMode,
	}
}
