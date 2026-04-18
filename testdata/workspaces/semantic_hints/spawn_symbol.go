package handler

// SpawnNamed launches a package-local function in a goroutine. The analyser
// should emit a SPAWNS hint that keeps the exact canonical fallback target.
func SpawnNamed(job string) {
	go runJob(job)
}

func runJob(job string) {
	_ = job
}
