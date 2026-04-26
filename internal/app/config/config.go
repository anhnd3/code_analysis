package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	ArtifactRoot  string
	SQLitePath    string
	HTTPAddr      string
	ProgressMode  string
	LLMBaseURL    string
	LLMModel      string
	LLMAPIKey     string
	LLMTimeoutSec int
	LLMMaxRetries int
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
	llmBaseURL := ""
	if value := os.Getenv("ANALYSIS_LLM_BASE_URL"); value != "" {
		llmBaseURL = value
	}
	llmModel := ""
	if value := os.Getenv("ANALYSIS_LLM_MODEL"); value != "" {
		llmModel = value
	}
	llmAPIKey := ""
	if value := os.Getenv("ANALYSIS_LLM_API_KEY"); value != "" {
		llmAPIKey = value
	}
	llmTimeoutSec := 15
	if value := os.Getenv("ANALYSIS_LLM_TIMEOUT_SEC"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			llmTimeoutSec = parsed
		}
	}
	llmMaxRetries := 2
	if value := os.Getenv("ANALYSIS_LLM_MAX_RETRIES"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			llmMaxRetries = parsed
		}
	}
	return Config{
		ArtifactRoot:  artifactRoot,
		SQLitePath:    sqlitePath,
		HTTPAddr:      addr,
		ProgressMode:  progressMode,
		LLMBaseURL:    llmBaseURL,
		LLMModel:      llmModel,
		LLMAPIKey:     llmAPIKey,
		LLMTimeoutSec: llmTimeoutSec,
		LLMMaxRetries: llmMaxRetries,
	}
}
