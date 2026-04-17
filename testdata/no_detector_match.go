// Package testdata contains fixture files used by boundary detector unit tests.
// This file deliberately contains NO framework route registrations. It is used in
// tests that verify the fallback (symbol-kind heuristic) logic in
// entrypoint_resolve.Service.Resolve IS invoked when no detector fires.
package testdata

// noDetectorMatch is a plain function with no HTTP or gRPC bindings.
// When run through the boundary detectors, it should produce zero roots,
// ensuring that the entrypoint resolver falls back to symbol-kind detection.
func noDetectorMatch() {
	doWork()
}

func doWork() {
	// Some internal computation — no routes, no handlers.
}
