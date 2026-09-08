package resolver

import (
	"context"
	"errors"
	"sort"

	"github.com/nabhold/baobab-cp/internal/domain"
)

// CapabilityBinding represents the effective binding between a capability and a runtime engine instance.
type CapabilityBinding = domain.CapabilityBinding

// CapabilityResolutionQuery resolves a capability in the current trusted context.
type CapabilityResolutionQuery struct {
	CapabilityKey string
	Context       Context
	Bindings      []CapabilityBinding
}

// ResolvedCapability is the selected capability binding and target engine.
type ResolvedCapability struct {
	CapabilityKey    string
	BindingMode      string
	EngineID         string
	EngineInstanceID string
	ContractVersion  string
}

// CapabilityResolverImpl resolves a capability to the highest-priority active binding.
type CapabilityResolverImpl struct{}

func (CapabilityResolverImpl) Resolve(_ context.Context, q CapabilityResolutionQuery) (ResolvedCapability, error) {
	if q.CapabilityKey == "" {
		return ResolvedCapability{}, errors.New("capability key is required")
	}
	if len(q.Bindings) == 0 {
		return ResolvedCapability{}, errors.New("capability not found")
	}

	active := make([]CapabilityBinding, 0, len(q.Bindings))
	for _, b := range q.Bindings {
		if b.CapabilityKey != q.CapabilityKey {
			continue
		}
		if b.Status != "ACTIVE" {
			continue
		}
		active = append(active, b)
	}
	if len(active) == 0 {
		return ResolvedCapability{}, errors.New("capability not found")
	}

	sort.Slice(active, func(i, j int) bool {
		if active[i].Priority != active[j].Priority {
			return active[i].Priority > active[j].Priority
		}
		if active[i].BindingMode != active[j].BindingMode {
			return active[i].BindingMode == "PRIMARY"
		}
		return active[i].EngineInstanceID > active[j].EngineInstanceID
	})

	chosen := active[0]
	return ResolvedCapability{
		CapabilityKey:    chosen.CapabilityKey,
		BindingMode:      chosen.BindingMode,
		EngineID:         chosen.EngineID,
		EngineInstanceID: chosen.EngineInstanceID,
		ContractVersion:  chosen.ContractVersion,
	}, nil
}
