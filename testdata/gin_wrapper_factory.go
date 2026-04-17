// Package testdata contains fixture files used by boundary detector unit tests.
// This file demonstrates Gin handler factory / middleware wrapper patterns that
// should be captured by the enhanced Gin detector.
package testdata

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Auth is a simple middleware/wrapper factory that returns a gin.HandlerFunc.
// Routes that use Auth() exercise the "wrapper call_expression" handler resolution path.
type Auth struct{ realm string }

func (a Auth) Required(inner gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ... auth check ...
		inner(c)
	}
}

// MakeOrderHandler is a handler factory function returning gin.HandlerFunc.
func MakeOrderHandler(repo interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ... handle order ...
	}
}

// SetupWrappedRoutes demonstrates the three wrapper/factory patterns the detector must handle:
//  1. Middleware wrapper method call:      r.GET("/profile",   auth.Required(profileHandler))
//  2. Handler factory call:               r.POST("/orders",   MakeOrderHandler(repo))
//  3. Dynamic path via fmt.Sprintf:       r.GET(dynamicPath,  listHandler)
func SetupWrappedRoutes(repo interface{}) {
	r := gin.New()
	auth := Auth{realm: "api"}

	// 1. Middleware wrapper — HandlerTarget should resolve to "auth.Required".
	r.GET("/profile", auth.Required(profileHandler))

	// 2. Handler factory — HandlerTarget should resolve to "MakeOrderHandler".
	r.POST("/orders", MakeOrderHandler(repo))

	// 3. Dynamic path — Path should be recorded as <dynamic:...>, confidence downgraded to medium.
	version := "v1"
	dynamicPath := fmt.Sprintf("/%s/items", version)
	r.GET(dynamicPath, listHandler)
}

func profileHandler(c *gin.Context) {}
func listHandler(c *gin.Context)    {}
