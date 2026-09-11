// Package domain holds the capability module's aggregate model: Capability,
// CapabilityScope, CapabilityGrant, CapabilityProvider,
// ProviderCapabilitySupport and CapabilityBinding (ADR-BCP-003 SS3,
// ADR-BCP-010 SS41). This package owns these types; other modules reference
// them through explicit imports rather than duplicating or reaching into
// this module's persistence directly (ADR-BCP-010 SS39-40, "one owning
// module per table/aggregate").
//
// Moved here from the former flat internal/domain package (where PR #83
// first landed CapabilityProvider/ProviderCapabilitySupport/CapabilityBinding
// ahead of this package split) to establish the ADR-BCP-010 SS41 module
// layout for the capability module before further capability-centric
// domain types (CapabilityScope, CapabilityGrant) are added.
package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var capabilityKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$`)

type Capability struct {
	ID          string `json:"id,omitempty"`
	Key         string `json:"capability_key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

func (c Capability) Validate() error {
	if !capabilityKeyPattern.MatchString(c.Key) || strings.TrimSpace(c.Name) == "" {
		return errors.New("capability requires an implementation-neutral key and name")
	}
	return nil
}

// BindingMode is the canonical, closed set of CapabilityBinding modes
// (BCP-TS-ONBOARDING-001 CR-002, mirrored in nabhold/shared's
// contracts/capability/v1/domain.schema.json). A superseded seven-value
// set (including SECONDARY, READ_ONLY, MIGRATION_SOURCE, MIGRATION_TARGET)
// existed in this repository's earlier migrations and must not be
// reintroduced -- see migration 000028's comment for the rationale and
// the remapping applied to any pre-existing rows.
type BindingMode string

const (
	// BindingModePrimary is the normal authoritative binding.
	BindingModePrimary BindingMode = "PRIMARY"
	// BindingModeFallback is eligible only under approved failover
	// semantics; it never bypasses entitlement, scope, contract,
	// residency or isolation checks.
	BindingModeFallback BindingMode = "FALLBACK"
	// BindingModeShadow is non-authoritative mirrored/validation
	// execution. A resolution SHALL NOT treat a SHADOW binding as a
	// valid authoritative result even if it wins ranking; a
	// SHADOW-only candidate set resolves as if no eligible binding
	// existed (nabhold/shared's scope-specificity.yaml, "binding mode
	// preference").
	BindingModeShadow BindingMode = "SHADOW"
	// BindingModeMigration is a temporary binding participating in an
	// explicit ProviderMigration plan.
	BindingModeMigration BindingMode = "MIGRATION"
	// BindingModeDisabled is retained but ineligible; it SHALL be
	// excluded from resolution candidates entirely, not merely
	// deprioritised.
	BindingModeDisabled BindingMode = "DISABLED"
)

// Valid reports whether m is one of the five canonical binding modes.
func (m BindingMode) Valid() bool {
	switch m {
	case BindingModePrimary, BindingModeFallback, BindingModeShadow, BindingModeMigration, BindingModeDisabled:
		return true
	default:
		return false
	}
}

// CapabilityProvider is a Baobab-recognised implementation of one or more
// capabilities. It is distinct from Engine (technology family) and
// EngineInstance (concrete deployment): a provider declares what it can
// implement, a binding declares where it does so
// (ADR-BCP-002 §5.3, ADR-BCP-006 §5-6, ADR-SHARED-007 §31-33). Confirmed
// entirely absent from this codebase during the Capability Platform
// Phase-0 audit -- this is net-new, not a remodel of an existing type.
type CapabilityProvider struct {
	ID           string         `json:"id,omitempty"`
	ProviderKey  string         `json:"provider_key"`
	Name         string         `json:"name"`
	ProviderType string         `json:"provider_type"`
	EngineID     string         `json:"engine_id"`
	Status       string         `json:"status"`
	Ownership    string         `json:"ownership,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Version      int64          `json:"version,omitempty"`
}

var providerKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`)

func (p CapabilityProvider) Validate() error {
	if !providerKeyPattern.MatchString(p.ProviderKey) {
		return errors.New("provider_key must be in <owning-repository>.<engine> form, e.g. baobab-trade.medusa")
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.EngineID) == "" {
		return errors.New("provider requires a name and engine_id")
	}
	switch p.ProviderType {
	case "BAOBAB_ENGINE", "EXTERNAL_SERVICE", "PLATFORM_NATIVE", "ADAPTER":
	default:
		return errors.New("provider_type must be one of BAOBAB_ENGINE, EXTERNAL_SERVICE, PLATFORM_NATIVE, ADAPTER")
	}
	return nil
}

// ProviderCapabilitySupport is a provider's explicit declaration that it
// implements a specific capability at specific contract versions. Support
// is never inferred merely from engine association (ADR-SHARED-007 §31).
type ProviderCapabilitySupport struct {
	ProviderID       string     `json:"provider_id"`
	CapabilityID     string     `json:"capability_id,omitempty"`
	CapabilityKey    string     `json:"capability_key,omitempty"`
	ContractVersions []int      `json:"contract_versions"`
	Status           string     `json:"status"`
	EffectiveFrom    time.Time  `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time `json:"effective_to,omitempty"`
}

func (s ProviderCapabilitySupport) Validate() error {
	if s.ProviderID == "" {
		return errors.New("provider_id is required")
	}
	if s.CapabilityID == "" && s.CapabilityKey == "" {
		return errors.New("capability identity is required")
	}
	if len(s.ContractVersions) == 0 {
		return errors.New("at least one contract_version is required")
	}
	if s.EffectiveTo != nil && !s.EffectiveTo.After(s.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	return nil
}

type CapabilityBinding struct {
	ID               string         `json:"id,omitempty"`
	CapabilityID     string         `json:"capability_id,omitempty"`
	CapabilityKey    string         `json:"capability_key,omitempty"`
	ProviderID       string         `json:"provider_id,omitempty"`
	EngineID         string         `json:"engine_id,omitempty"`
	EngineInstanceID string         `json:"engine_instance_id"`
	ScopeID          string         `json:"scope_id"`
	BindingMode      BindingMode    `json:"binding_mode"`
	Priority         int            `json:"priority"`
	Status           string         `json:"status"`
	ContractVersion  string         `json:"contract_version"`
	EffectiveFrom    time.Time      `json:"effective_from,omitempty"`
	EffectiveTo      *time.Time     `json:"effective_to,omitempty"`
	Configuration    map[string]any `json:"configuration,omitempty"`
	Version          int64          `json:"version,omitempty"`
}

func (b CapabilityBinding) Validate() error {
	if b.CapabilityID == "" && b.CapabilityKey == "" {
		return errors.New("capability identity is required")
	}
	if b.EngineInstanceID == "" || b.ScopeID == "" {
		return errors.New("engine_instance_id and scope_id are required")
	}
	if b.BindingMode != "" && !b.BindingMode.Valid() {
		return errors.New("binding_mode must be one of PRIMARY, FALLBACK, SHADOW, MIGRATION, DISABLED")
	}
	if b.EffectiveTo != nil && !b.EffectiveTo.After(b.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	return nil
}
