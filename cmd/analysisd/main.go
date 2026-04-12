package main

import (
	"net/http"

	"analysis-module/internal/app/bootstrap"
	"analysis-module/internal/app/config"
	"analysis-module/internal/app/logging"
)

func main() {
	logger := logging.New()
	cfg := config.Default()
	app, err := bootstrap.New(cfg, logger)
	if err != nil {
		logger.Error("bootstrap failed", "error", err)
		panic(err)
	}
	logger.Info("analysisd listening", "addr", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, app.HTTPHandler); err != nil {
		logger.Error("server failed", "error", err)
		panic(err)
	}
}
