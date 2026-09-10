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

type CapabilityBinding struct {
	ID               string         `json:"id,omitempty"`
	CapabilityID     string         `json:"capability_id,omitempty"`
	CapabilityKey    string         `json:"capability_key,omitempty"`
	EngineID         string         `json:"engine_id,omitempty"`
	EngineInstanceID string         `json:"engine_instance_id"`
	ScopeID          string         `json:"scope_id"`
	BindingMode      string         `json:"binding_mode"`
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
	if b.EffectiveTo != nil && !b.EffectiveTo.After(b.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	return nil
}

// MappingScope's fields mirror nabhold/shared's contracts/control-plane/v1/
// canonical-mapping.schema.json #/$defs/mappingScope field-for-field (names,
// required-ness) per docs/reconciliation/platform-resolution-spine-audit.md
// Gate 1; see internal/domain/contract_compatibility_test.go. Nothing in
// this repository currently loads a MappingScope from Postgres (see
// internal/repository/postgres.go's ListMappings: the only mapping.
// mapping_scope column read is mapping_scope_id, joined in purely to supply
// Mapping.ScopeID) -- this type is constructed in-memory/by tests only, so
// this reconciliation needed no accompanying migration.
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
