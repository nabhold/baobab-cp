package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var capabilityKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$`)

type TrustLevel string

const (
	TrustUntrusted  TrustLevel = "UNTRUSTED"
	TrustVerified   TrustLevel = "VERIFIED"
	TrustAuthorised TrustLevel = "AUTHORISED"
	TrustSystem     TrustLevel = "SYSTEM"
)

type ContextSource struct {
	Source     string     `json:"source"`
	TrustLevel TrustLevel `json:"trust_level"`
	Evidence   string     `json:"evidence,omitempty"`
}

// Context is an immutable, server-resolved operation scope. Distinct IDs are
// deliberately separate: none is an alias for another.
type Context struct {
	ID                 string                   `json:"id,omitempty"`
	PrincipalID        string                   `json:"principal_id"`
	TenantID           string                   `json:"tenant_id"`
	LegalEntityID      string                   `json:"legal_entity_id,omitempty"`
	DigitalEstateID    string                   `json:"digital_estate_id,omitempty"`
	DigitalPropertyID  string                   `json:"digital_property_id,omitempty"`
	ChannelID          string                   `json:"channel_id,omitempty"`
	MarketID           string                   `json:"market_id,omitempty"`
	Jurisdiction       string                   `json:"jurisdiction,omitempty"`
	CountryCode        string                   `json:"country_code,omitempty"`
	CurrencyCode       string                   `json:"currency_code,omitempty"`
	Locale             string                   `json:"locale,omitempty"`
	DeploymentRegion   string                   `json:"deployment_region,omitempty"`
	Environment        string                   `json:"environment,omitempty"`
	IsolationProfileID string                   `json:"isolation_profile_id,omitempty"`
	CorrelationID      string                   `json:"correlation_id"`
	ResolvedAt         time.Time                `json:"resolved_at"`
	Provenance         map[string]ContextSource `json:"provenance"`
}

func (c Context) Validate() error {
	if strings.TrimSpace(c.PrincipalID) == "" || strings.TrimSpace(c.TenantID) == "" {
		return errors.New("principal_id and tenant_id are required")
	}
	if strings.TrimSpace(c.CorrelationID) == "" || c.ResolvedAt.IsZero() {
		return errors.New("correlation_id and resolved_at are required")
	}
	for field, source := range c.Provenance {
		if field == "" || source.Source == "" || source.TrustLevel == "" || source.TrustLevel == TrustUntrusted {
			return errors.New("context provenance must identify a trusted source")
		}
	}
	return nil
}

type IsolationProfile struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name"`
	Strategy         string `json:"strategy"`
	TenantScope      string `json:"tenant_scope"`
	DataPartitioning string `json:"data_partitioning"`
	IsDefault        bool   `json:"is_default"`
}

type Engine struct {
	ID          string `json:"id,omitempty"`
	Key         string `json:"engine_key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

type EngineInstance struct {
	ID                 string     `json:"id,omitempty"`
	EngineID           string     `json:"engine_id"`
	Key                string     `json:"engine_instance_key,omitempty"`
	Region             string     `json:"region"`
	Environment        string     `json:"environment"`
	IsolationProfileID string     `json:"isolation_profile_id,omitempty"`
	ResidencyRegion    string     `json:"residency_region,omitempty"`
	Status             string     `json:"status"`
	HealthStatus       string     `json:"health_status,omitempty"`
	EffectiveFrom      time.Time  `json:"effective_from,omitempty"`
	EffectiveTo        *time.Time `json:"effective_to,omitempty"`
}

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

// MappingScope's fields mirror nabhold/shared's contracts/control-plane/v1/
// canonical-mapping.schema.json #/$defs/mappingScope field-for-field (names,
// required-ness) per docs/reconciliation/platform-resolution-spine-audit.md
// Gate 1; see internal/domain/contract_compatibility_test.go. Gate 2
// completed mapping.mapping_scope's schema and added real Postgres-backed
// persistence (internal/repository/postgres.go's CreateMappingScope/
// GetMappingScope/ListMappingScopes) -- MarketID, DigitalPropertyID,
// EngineID and EngineInstanceID still read back as this database's own
// uuid identifiers rather than the wire schema's slug pattern; see
// migration 000025_mapping_scope_dimensions.sql for why that particular
// gap is left open rather than converted unilaterally.
type MappingScope struct {
	ScopeID            string   `json:"scope_id,omitempty"`
	TenantID           string   `json:"tenant_id,omitempty"`
	LegalEntityID      string   `json:"legal_entity_id,omitempty"`
	OrganisationID     string   `json:"organisation_id,omitempty"`
	BusinessUnitID     string   `json:"business_unit_id,omitempty"`
	OperatingRegionID  string   `json:"operating_region_id,omitempty"`
	GeographicRegionID string   `json:"geographic_region_id,omitempty"`
	MarketID           string   `json:"market_id,omitempty"`
	Country            string   `json:"country,omitempty"`
	EstateID           string   `json:"estate_id,omitempty"`
	DigitalPropertyID  string   `json:"digital_property_id,omitempty"`
	ChannelID          string   `json:"channel_id,omitempty"`
	Currency           string   `json:"currency,omitempty"`
	Locale             string   `json:"locale,omitempty"`
	CatalogueID        string   `json:"catalogue_id,omitempty"`
	CustomerSegmentID  string   `json:"customer_segment_id,omitempty"`
	EngineID           string   `json:"engine_id,omitempty"`
	EngineInstanceID   string   `json:"engine_instance_id,omitempty"`
	Environment        string   `json:"environment,omitempty"`
	DeploymentRegion   string   `json:"deployment_region,omitempty"`
	IncludeCountries   []string `json:"include_countries,omitempty"`
	ExcludeCountries   []string `json:"exclude_countries,omitempty"`
	CreatedAt          string   `json:"created_at,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

func (s MappingScope) Validate() error {
	if strings.TrimSpace(s.ScopeID) == "" {
		return errors.New("scope_id is required")
	}
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	return nil
}

type ExternalReference struct {
	ID                string         `json:"id,omitempty"`
	CanonicalEntityID string         `json:"canonical_entity_id"`
	EngineID          string         `json:"engine_id"`
	EngineInstanceID  string         `json:"engine_instance_id,omitempty"`
	NativeType        string         `json:"native_type"`
	NativeID          string         `json:"native_id"`
	ExternalURL       string         `json:"external_url,omitempty"`
	Status            string         `json:"status"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

func (r ExternalReference) Validate() error {
	if r.CanonicalEntityID == "" || r.EngineID == "" || r.NativeType == "" || r.NativeID == "" {
		return errors.New("canonical_entity_id, engine_id, native_type and native_id are required")
	}
	return nil
}
