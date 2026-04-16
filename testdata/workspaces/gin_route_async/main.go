package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	r := gin.Default()
	r.GET("/async", func(c *gin.Context) {
		go func() {
			// async work
		}()
		c.JSON(http.StatusOK, gin.H{"status": "accepted"})
	})
	r.Run()
}
