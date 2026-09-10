package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/resolver"
)

// ResolutionRequest is the service-level input for a control-plane resolution request.
//
// TenantID and CanonicalEntityID stay distinct end to end (ADR-0005 §2) --
// see resolver.ResolutionRequest's doc comment for why conflating them
// (docs/governance/gate-iam-0-discovery.md R-3) was a real bug, not a
// naming choice.
type ResolutionRequest struct {
	TenantID          string
	CanonicalEntityID string
	Context           resolver.Context
	Mappings          []domain.Mapping
	Bindings          []resolver.CapabilityBinding
	EngineInstances   []resolver.EngineInstance
}

// ResolutionResult contains the fully resolved runtime decision for a tenant request.
type ResolutionResult struct {
	Context    resolver.Context
	Mapping    resolver.ResolvedMapping
	Capability resolver.ResolvedCapability
	Policy     resolver.PolicyDecision
	Topology   resolver.EngineInstance
	Trace      resolver.ResolutionTrace
}

// ResolutionService exposes the composed resolver pipeline as a service interface.
type ResolutionService struct {
	Pipeline   resolver.ResolutionPipeline
	Repository repository.ResolverRepository
}

func (s ResolutionService) Resolve(ctx context.Context, req ResolutionRequest) (ResolutionResult, error) {
	if ctx == nil {
		return ResolutionResult{}, errors.New("context is required")
	}
	if req.TenantID == "" && req.Context.TenantID == "" {
		return ResolutionResult{}, errors.New("tenant_id is required")
	}
	if req.Context.TenantID == "" {
		req.Context.TenantID = req.TenantID
	}
	if req.CanonicalEntityID == "" {
		return ResolutionResult{}, errors.New("canonical_entity_id is required")
	}
	if s.Repository != nil {
		mappings, err := s.Repository.ListMappings(ctx, req.CanonicalEntityID)
		if err != nil {
			return ResolutionResult{}, fmt.Errorf("load mappings: %w", err)
		}
		bindings, err := s.Repository.ListBindings(ctx, "baobab_trade")
		if err != nil {
			return ResolutionResult{}, fmt.Errorf("load capability bindings: %w", err)
		}
		var instances []resolver.EngineInstance
		if len(bindings) > 0 {
			instances, err = s.Repository.ListActiveInstances(ctx, bindings[0].EngineID)
			if err != nil {
				return ResolutionResult{}, fmt.Errorf("load engine instances: %w", err)
			}
		}
		req.Mappings, req.Bindings, req.EngineInstances = mappings, bindings, instances
	}

	pipelineResult, err := s.Pipeline.Resolve(ctx, resolver.ResolutionRequest{
		TenantID:          req.TenantID,
		CanonicalEntityID: req.CanonicalEntityID,
		Context:           req.Context,
		Candidates:        req.Mappings,
		Bindings:          req.Bindings,
		EngineInstances:   req.EngineInstances,
	})
	if err != nil {
		return ResolutionResult{}, fmt.Errorf("resolution failed: %w", err)
	}

	return ResolutionResult{
		Context:    pipelineResult.Context,
		Mapping:    pipelineResult.Mapping,
		Capability: pipelineResult.Capability,
		Policy:     pipelineResult.Policy,
		Topology:   pipelineResult.Topology,
		Trace:      pipelineResult.Trace,
	}, nil
}
