package handler

// SpawnWorker launches an anonymous goroutine. The analyser should emit a
// SPAWNS hint for the go statement.
func SpawnWorker(jobs <-chan string) {
	go func() {
		for job := range jobs {
			_ = job
		}
	}()
}
