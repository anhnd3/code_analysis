package flow

// Resolver resolves dependencies for flow tracing.
type Resolver interface {
	Resolve(request FlowTraceRequest) (FlowPack, error)
}
