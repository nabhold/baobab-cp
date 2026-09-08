package resolver

import (
	"context"
	"errors"
	"testing"
)

func TestResolutionTraceExplainsSuccessfulRoute(t *testing.T) {
	req := resilientResolutionRequest()
	req.Context.CorrelationID = "corr-operator-1"
	result, err := (ResolutionPipeline{}).Resolve(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	trace := result.Trace
	if trace.Outcome != "ROUTED" || trace.TenantID != "tn_zuribeans" || trace.MappingID != "mapping-1" || trace.BindingID != "binding-1" || trace.EngineInstanceID != "instance-1" {
		t.Fatalf("incomplete route explanation: %#v", trace)
	}
}

func TestResolutionErrorCarriesFailureReasonAndContext(t *testing.T) {
	req := resilientResolutionRequest()
	req.Context.CorrelationID = "corr-operator-2"
	req.EngineInstances[0].Status = "RETIRED"
	_, err := (ResolutionPipeline{}).Resolve(context.Background(), req)
	var resolutionErr *ResolutionError
	if !errors.As(err, &resolutionErr) {
		t.Fatalf("expected typed resolution error, got %v", err)
	}
	trace := resolutionErr.Trace
	if trace.Outcome != "FAILED" || trace.Reason == "" || trace.CorrelationID != "corr-operator-2" || trace.MappingID != "mapping-1" || trace.BindingID != "binding-1" || trace.EngineInstanceID != "instance-1" {
		t.Fatalf("incomplete failure explanation: %#v", trace)
	}
}
