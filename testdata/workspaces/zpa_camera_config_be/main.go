package main

import (
    "github.com/gin-gonic/gin"
    "net/http"
)

type CameraController struct{}

func (ctrl *CameraController) GetConfig(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"config": "default"})
}

func main() {
    r := gin.Default()
    ctrl := &CameraController{}
    
    api := r.Group("/api/v1")
    {
        api.GET("/camera/config", ctrl.GetConfig)
    }
    
    r.Run()
}
