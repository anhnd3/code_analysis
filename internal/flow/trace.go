package flow

type TraceService interface {
	Trace(request FlowTraceRequest) (FlowPack, error)
}
