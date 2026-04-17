package handler

import "fmt"

// DeferCleanup registers a deferred function call. The analyser should emit a
// DEFERS hint for the defer statement.
func DeferCleanup(resource string) {
	defer fmt.Println("cleanup:", resource)
	fmt.Println("working with:", resource)
}
