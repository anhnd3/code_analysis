package trace

type TraceService interface {
	Trace(request FlowTraceRequest) (FlowPack, error)
}