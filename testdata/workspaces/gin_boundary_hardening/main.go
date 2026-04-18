package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type fakeBuilder struct{}

func (fakeBuilder) POST(string, interface{}) {}

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func configFactory() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"config": "all"})
	}
}

func detectQRHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"version": "v1"})
	}
}

func detectQRV2(c *gin.Context) {
	c.JSON(http.StatusAccepted, gin.H{"version": "v2"})
}

func abtest(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"abtest": true})
}

func main() {
	r := gin.Default()
	api := r

	api.GET("/health", health)

	v1 := api.Group("/v1")
	cameraV1 := v1.Group("/camera")
	cameraV1.GET("/config/all", configFactory())
	cameraV1.POST("/detect-qr", detectQRHandler())
	cameraV1.GET("/abtest", abtest)

	v2 := r.Group("/v2")
	cameraV2 := v2.Group("/camera")
	cameraV2.POST("/detect-qr", detectQRV2)

	r.Group("/metrics").GET("/prom", gin.WrapH(promhttp.Handler()))

	logger := zap.NewNop()
	logger.Any("zlp_token", "secret")
	logger.Any("error", http.ErrServerClosed)

	other := fakeBuilder{}
	other.POST("/detect-qr", detectQRV2)

	r.Run()
}
