package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	r := gin.Default()
	r.POST("/process", func(c *gin.Context) {
		items := []string{"a", "b"}
		for _, item := range items {
			if item == "a" {
				// branch A
			} else {
				// branch B
			}
		}
		c.Status(http.StatusAccepted)
	})
	r.Run()
}
