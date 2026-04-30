package handler

func SpawnWorker() {
	go func() {
		doWork()
	}()
}

func doWork() {}
