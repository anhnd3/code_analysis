// Package testdata contains fixture files used by boundary detector unit tests.
// This file demonstrates overlapping net/http and Gin route registrations on the
// same path. The deduplication logic in registry.DetectAll should produce a single
// root for the shared route, choosing the higher-confidence Gin detector.
package testdata

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupOverlappingRoutes registers the same logical endpoint via both net/http and Gin.
// In practice this would not compile/run as a valid server, but it is valid Go for
// static analysis; it exercises the two detectors simultaneously.
func SetupOverlappingRoutes() {
	// net/http registers /api/users — picked up by NetHTTPDetector (confidence: medium).
	http.HandleFunc("/api/users", getUserHTTP)

	// Gin also registers /api/users — picked up by GinDetector (confidence: high).
	r := gin.New()
	r.GET("/api/users", getUserGin)

	// A Gin-only route — no net/http counterpart, no deduplication needed.
	r.POST("/api/users", createUserGin)
}

func getUserHTTP(w http.ResponseWriter, req *http.Request) {}
func getUserGin(c *gin.Context)                            {}
func createUserGin(c *gin.Context)                         {}
