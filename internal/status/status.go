// Package status contains the application boundary for workstation status.
package status

// Probe is the semantic outbound boundary used to observe endpoint reachability.
type Probe interface {
	Probe() error
}

// Result is the immediate result of a workstation status request.
type Result string

const (
	// Online reports that the configured endpoint accepted a probe connection.
	Online Result = "online"
	// Offline reports that the configured endpoint could not be probed.
	Offline Result = "offline"
)

// UseCase is the application behavior for a status request.
type UseCase struct {
	probe Probe
}

// New creates a status use case using probe.
func New(probe Probe) *UseCase {
	return &UseCase{probe: probe}
}

// Status reports the immediate workstation status.
//
// Status behavior is implemented in the following production phase.
func (u *UseCase) Status() Result {
	return ""
}
