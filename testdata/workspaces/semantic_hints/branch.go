package handler

// Processor is a simple content-type discriminator.
type Processor struct{ kind string }

// RouteProcessor iterates over a slice of processors and dispatches by kind.
// The analyser should emit HintBranch hints for the if/else inside the loop.
func RouteProcessor(processors []Processor, input string) string {
	for _, p := range processors {
		if p.kind == "json" {
			return "json:" + input
		} else if p.kind == "deeplink" {
			return "deeplink:" + input
		} else {
			return "fallback:" + input
		}
	}
	return ""
}
