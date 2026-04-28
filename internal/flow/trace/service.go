package trace

import flowmodel "analysis-module/internal/flow/model"

type TraceService interface {
	Trace(request flowmodel.FlowTraceRequest) (flowmodel.FlowPack, error)
}
