package handler

// ReturnHandlerFunc returns a closure. The analyser should detect the
// returned func literal and emit a RETURNS_HANDLER hint.
func ReturnHandlerFunc() func(string) string {
	return func(input string) string {
		return "handled: " + input
	}
}
