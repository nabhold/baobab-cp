package resolver

import (
	"context"
	"errors"
	"sort"
	"time"

	capabilitydomain "github.com/nabhold/baobab-cp/internal/capability/domain"
	"github.com/nabhold/baobab-cp/internal/domain"
)

// CapabilityBinding represents the effective binding between a capability and a runtime engine instance.
type CapabilityBinding = capabilitydomain.CapabilityBinding

// CapabilityResolutionQuery resolves a capability in the current trusted context.
type CapabilityResolutionQuery struct {
	CapabilityKey string
	Context       Context
	Bindings      []CapabilityBinding
	Scopes        map[string]domain.MappingScope
	At            time.Time
}

// ResolvedCapability is the selected capability binding and target engine.
type ResolvedCapability struct {
	BindingID        string
	CapabilityKey    string
	BindingMode      string
	EngineID         string
	EngineInstanceID string
	ContractVersion  string
	Specificity      int
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

	at := q.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	type rankedBinding struct {
		binding     CapabilityBinding
		specificity int
	}
	active := make([]rankedBinding, 0, len(q.Bindings))
	for _, b := range q.Bindings {
		if b.CapabilityKey != q.CapabilityKey {
			continue
		}
		if b.Status != "ACTIVE" {
			continue
		}
		// DISABLED bindings are excluded from resolution candidates
		// entirely, not merely deprioritised (capabilitydomain.BindingModeDisabled).
		if b.BindingMode == capabilitydomain.BindingModeDisabled {
			continue
		}
		if !b.EffectiveFrom.IsZero() && at.Before(b.EffectiveFrom) {
			continue
		}
		if b.EffectiveTo != nil && !at.Before(*b.EffectiveTo) {
			continue
		}
		specificity := 0
		if len(q.Scopes) > 0 {
			scope, ok := q.Scopes[b.ScopeID]
			if !ok {
				continue
			}
			match := DefaultScopeMatcher{}.Match(q.Context, scope)
			if !match.Compatible {
				continue
			}
			specificity = match.Specificity
		}
		active = append(active, rankedBinding{binding: b, specificity: specificity})
	}
	if len(active) == 0 {
		return ResolvedCapability{}, errors.New("capability not found")
	}

	sort.Slice(active, func(i, j int) bool {
		if active[i].specificity != active[j].specificity {
			return active[i].specificity > active[j].specificity
		}
		if active[i].binding.Priority != active[j].binding.Priority {
			return active[i].binding.Priority > active[j].binding.Priority
		}
		if active[i].binding.BindingMode != active[j].binding.BindingMode {
			return bindingModeRank(active[i].binding.BindingMode) > bindingModeRank(active[j].binding.BindingMode)
		}
		return active[i].binding.ID < active[j].binding.ID
	})

	if len(active) > 1 && active[0].specificity == active[1].specificity &&
		active[0].binding.Priority == active[1].binding.Priority &&
		bindingModeRank(active[0].binding.BindingMode) == bindingModeRank(active[1].binding.BindingMode) {
		return ResolvedCapability{}, errors.New("capability binding is ambiguous")
	}
	chosen := active[0]
	// A SHADOW binding is non-authoritative: even when it wins ranking (no
	// PRIMARY/FALLBACK/MIGRATION candidate is eligible), it SHALL NOT be
	// returned as a resolution result -- resolve as if no eligible binding
	// existed (capabilitydomain.BindingModeShadow; nabhold/shared's
	// scope-specificity.yaml "binding mode preference").
	if chosen.binding.BindingMode == capabilitydomain.BindingModeShadow {
		return ResolvedCapability{}, errors.New("capability not found")
	}
	return ResolvedCapability{
		BindingID:        chosen.binding.ID,
		CapabilityKey:    chosen.binding.CapabilityKey,
		BindingMode:      string(chosen.binding.BindingMode),
		EngineID:         chosen.binding.EngineID,
		EngineInstanceID: chosen.binding.EngineInstanceID,
		ContractVersion:  chosen.binding.ContractVersion,
		Specificity:      chosen.specificity,
	}, nil
}

// bindingModeRank orders binding modes by resolution preference (highest
// first): PRIMARY, FALLBACK, SHADOW, MIGRATION -- mirroring nabhold/shared's
// contracts/capability/v1/scope-specificity.yaml "binding_mode_preference".
// DISABLED is never ranked: it is filtered out of candidates before this is
// ever consulted.
func bindingModeRank(mode capabilitydomain.BindingMode) int {
	switch mode {
	case capabilitydomain.BindingModePrimary:
		return 4
	case capabilitydomain.BindingModeFallback:
		return 3
	case capabilitydomain.BindingModeShadow:
		return 2
	case capabilitydomain.BindingModeMigration:
		return 1
	default:
		return 0
	}
}
