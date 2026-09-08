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
}

// ResolutionPipeline composes the resolution stack into a single deterministic decision process.
type ResolutionPipeline struct{}

func (ResolutionPipeline) Resolve(ctx context.Context, req ResolutionRequest) (ResolutionResult, error) {
	if req.TenantID == "" && req.Context.TenantID == "" {
		return ResolutionResult{}, errors.New("tenant context required")
	}
	if req.Context.TenantID == "" {
		req.Context.TenantID = req.TenantID
	}

	if req.Context.Provenance == nil {
		req.Context.Provenance = map[string]ContextSource{}
	}

	mappingResult, err := MappingResolverImpl{}.Resolve(ctx, MappingResolutionQuery{
		CanonicalEntityID: req.TenantID,
		Context:           req.Context,
		Candidates:        req.Candidates,
	})
	if err != nil {
		return ResolutionResult{}, err
	}

	capabilityResult, err := CapabilityResolverImpl{}.Resolve(ctx, CapabilityResolutionQuery{
		CapabilityKey: "baobab_trade",
		Context:       req.Context,
		Bindings:      req.Bindings,
	})
	if err != nil {
		return ResolutionResult{}, err
	}

	topologyResult, err := TopologyResolverImpl{}.Resolve(ctx, TopologyResolutionQuery{
		Context:                  req.Context,
		SelectedEngineInstanceID: capabilityResult.EngineInstanceID,
		EngineInstances:          req.EngineInstances,
		At:                       req.Context.ResolvedAt,
	})
	if err != nil {
		return ResolutionResult{}, err
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
		return ResolutionResult{}, errors.New(policyResult.Reason)
	}

	return ResolutionResult{
		Context:    req.Context,
		Mapping:    mappingResult,
		Capability: capabilityResult,
		Policy:     policyResult,
		Topology:   topologyResult,
	}, nil
}
