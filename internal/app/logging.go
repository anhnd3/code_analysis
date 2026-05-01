package app

import (
	"log/slog"
	"os"
)

// NewLogger returns a new slog.Logger writing to stderr with INFO level.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
