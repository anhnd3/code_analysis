package flow

// Status represents the verification status of a step.
type Status string

const (
	StatusAccepted  Status = "accepted"
	StatusAmbiguous Status = "ambiguous"
	StatusRejected  Status = "rejected"
)

// Verifier verifies flow steps and returns their status.
type Verifier interface {
	Verify(step FlowNode) (Status, error)
}
