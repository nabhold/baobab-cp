package resolver

import "fmt"

type ResolutionTrace struct {
	CorrelationID    string
	TenantID         string
	CapabilityKey    string
	MappingID        string
	GrantID          string
	BindingID        string
	EngineInstanceID string
	Outcome          string
	Reason           string
}

type ResolutionError struct {
	Trace ResolutionTrace
	Cause error
}

func (e *ResolutionError) Error() string {
	return fmt.Sprintf("resolution %s: %v", e.Trace.Outcome, e.Cause)
}

func (e *ResolutionError) Unwrap() error { return e.Cause }

func resolutionFailure(trace ResolutionTrace, cause error) error {
	trace.Outcome = "FAILED"
	trace.Reason = cause.Error()
	return &ResolutionError{Trace: trace, Cause: cause}
}
