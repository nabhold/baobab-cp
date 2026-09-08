package resolver

import (
	"context"
	"errors"

	"github.com/nabhold/baobab-cp/internal/domain"
)

// ResolutionRequest is the combined request used by the full resolver pipeline.
type ResolutionRequest struct {
	TenantID        string
	Context         Context
	Candidates      []domain.Mapping
	Bindings        []CapabilityBinding
	EngineInstances []EngineInstance
}

// ResolutionResult is the final output from the composed resolver pipeline.
type ResolutionResult struct {
	Context    Context
	Mapping    ResolvedMapping
	Capability ResolvedCapability
	Policy     PolicyDecision
	Topology   EngineInstance
	Trace      ResolutionTrace
}

// ResolutionPipeline composes the resolution stack into a single deterministic decision process.
type ResolutionPipeline struct{}

func (ResolutionPipeline) Resolve(ctx context.Context, req ResolutionRequest) (ResolutionResult, error) {
	trace := ResolutionTrace{CorrelationID: req.Context.CorrelationID, TenantID: req.Context.TenantID, CapabilityKey: "baobab_trade"}
	if req.TenantID == "" && req.Context.TenantID == "" {
		return ResolutionResult{}, resolutionFailure(trace, errors.New("tenant context required"))
	}
	if req.Context.TenantID == "" {
		req.Context.TenantID = req.TenantID
	}
	trace.TenantID = req.Context.TenantID

	if req.Context.Provenance == nil {
		req.Context.Provenance = map[string]ContextSource{}
	}

	mappingResult, err := MappingResolverImpl{}.Resolve(ctx, MappingResolutionQuery{
		CanonicalEntityID: req.TenantID,
		Context:           req.Context,
		Candidates:        req.Candidates,
	})
	if err != nil {
		return ResolutionResult{}, resolutionFailure(trace, err)
	}
	trace.MappingID = mappingResult.Mapping.ID

	capabilityResult, err := CapabilityResolverImpl{}.Resolve(ctx, CapabilityResolutionQuery{
		CapabilityKey: "baobab_trade",
		Context:       req.Context,
		Bindings:      req.Bindings,
	})
	if err != nil {
		return ResolutionResult{}, resolutionFailure(trace, err)
	}
	trace.BindingID = capabilityResult.BindingID
	trace.EngineInstanceID = capabilityResult.EngineInstanceID

	topologyResult, err := TopologyResolverImpl{}.Resolve(ctx, TopologyResolutionQuery{
		Context:                  req.Context,
		SelectedEngineInstanceID: capabilityResult.EngineInstanceID,
		EngineInstances:          req.EngineInstances,
		At:                       req.Context.ResolvedAt,
	})
	if err != nil {
		return ResolutionResult{}, resolutionFailure(trace, err)
	}

	policyResult := PolicyChecker{}.Check(ctx, req.Context, CapabilityBinding{
		CapabilityKey:    capabilityResult.CapabilityKey,
		EngineID:         capabilityResult.EngineID,
		EngineInstanceID: capabilityResult.EngineInstanceID,
		BindingMode:      capabilityResult.BindingMode,
		Status:           "ACTIVE",
		ContractVersion:  capabilityResult.ContractVersion,
	})
	if !policyResult.Allowed {
		return ResolutionResult{}, resolutionFailure(trace, errors.New(policyResult.Reason))
	}
	trace.Outcome = "ROUTED"
	trace.Reason = "active mapping and binding selected; policy allowed; engine instance eligible"

	return ResolutionResult{
		Context:    req.Context,
		Mapping:    mappingResult,
		Capability: capabilityResult,
		Policy:     policyResult,
		Topology:   topologyResult,
		Trace:      trace,
	}, nil
}
