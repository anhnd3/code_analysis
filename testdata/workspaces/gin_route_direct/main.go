package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func directHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

func main() {
	r := gin.Default()
	r.GET("/ping", directHandler)
	r.Run()
}
