package handler

import "sync"

// WaitGroupJoin uses wg.Wait() to synchronise goroutines. The analyser should
// emit a WAITS_ON hint for the Wait call.
func WaitGroupJoin(items []string) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			_ = s
		}(item)
	}
	wg.Wait()
}
