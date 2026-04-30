package handler

import (
	"sync"
)

func WaitGroupJoin() {
	var wg sync.WaitGroup

	go func() { taskA() }()

	go func() { taskB() }()

	wg.Wait()
}

func taskA()      {}
func taskB(_ int) {}
