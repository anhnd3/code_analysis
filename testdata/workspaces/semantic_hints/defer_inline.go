package handler

// DeferInline registers a deferred inline closure. The analyser should emit a
// DEFERS hint bound to the synthetic inline symbol.
func DeferInline(resource string) {
	defer func() {
		_ = resource
	}()
}
