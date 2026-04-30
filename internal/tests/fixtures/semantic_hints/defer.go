package handler

import "fmt"

func DeferCleanup() {
	defer fmt.Println("cleanup")
}
