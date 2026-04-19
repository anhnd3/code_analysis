package cmd

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type handlers struct{}

func (handlers) DetectQR(*gin.Context) {}

func (handlers) GetConfigCameraZPA() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.WriteHeader(200)
	}
}

func (handlers) DetectQRHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.WriteHeader(200)
	}
}

func (s *service) withRouter() {
	engine := s.Engine()
	engine.GET("/health", func(c *gin.Context) { c.Writer.WriteHeader(200) })
	engine.GET("/metrics/prom", gin.WrapH(promhttp.Handler()))

	api := engine
	v1 := api.Group("/v1")
	cameraV1 := v1.Group("/camera")
	h := handlers{}
	cameraV1.GET("/config/all", h.GetConfigCameraZPA())
	cameraV1.POST("/detect-qr", h.DetectQRHandler())

	v2 := engine.Group("/v2")
	cameraV2 := v2.Group("/camera")
	cameraV2.POST("/detect-qr", h.DetectQR)
}
