package resolver

import (
	"context"
	"errors"

	"github.com/nabhold/baobab-cp/internal/domain"
)

// ResolutionRequest is the combined request used by the full resolver pipeline.
//
// TenantID and CanonicalEntityID are deliberately separate fields (ADR-0005
// §2, "Realm/Org/Tenant/LegalEntity Model"): a tenant is the platform
// boundary a request executes within, a canonical entity is the specific
// business object (product, customer, order, ...) a mapping resolves for.
// Conflating them here (docs/governance/gate-iam-0-discovery.md R-3) meant
// /v1/resolve could only ever "resolve the tenant itself" instead of an
// actual entity within it -- see Resolve's mapping-lookup call below.
type ResolutionRequest struct {
	TenantID          string
	CanonicalEntityID string
	Context           Context
	Candidates        []domain.Mapping
	Bindings          []CapabilityBinding
	EngineInstances   []EngineInstance
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

	if req.CanonicalEntityID == "" {
		return ResolutionResult{}, resolutionFailure(trace, errors.New("canonical_entity_id is required"))
	}

	if req.Context.Provenance == nil {
		req.Context.Provenance = map[string]ContextSource{}
	}

	mappingResult, err := MappingResolverImpl{}.Resolve(ctx, MappingResolutionQuery{
		CanonicalEntityID: req.CanonicalEntityID,
		Context:           req.Context,
		Candidates:        req.Candidates,
	})
	if err != nil {
		return ResolutionResult{}, resolutionFailure(trace, err)
	}
	// domain.Mapping.Validate() (called inside resolveMapping's eligibility
	// filter) already requires TenantID to be non-empty, so this is a
	// straight equality check, not a nil-guarded one: a resolved mapping
	// whose own tenant doesn't match the requesting tenant is exactly the
	// cross-tenant leakage ADR-0005's separation exists to prevent, and
	// must fail closed rather than silently route through.
	if mappingResult.Mapping.TenantID != req.Context.TenantID {
		return ResolutionResult{}, resolutionFailure(trace, errors.New("mapping tenant does not match request tenant"))
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
