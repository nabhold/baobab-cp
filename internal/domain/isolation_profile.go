package domain

import (
	"errors"
	"strings"
	"time"
)

// Validate mirrors policy.isolation_profile's own CHECK constraint
// (migration 000004): strategy must be one of the two values the table
// accepts.
func (p IsolationProfile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if p.Strategy != "schema_per_tenant" && p.Strategy != "row_level_security" {
		return errors.New("strategy must be schema_per_tenant or row_level_security")
	}
	return nil
}

// TenantIsolationProfileAssignment models policy.tenant_isolation_profile
// (migration 000004, extended by migration 000024 with a generated
// valid_period column and a tenant_isolation_profile_active_excl exclusion
// constraint). Unlike MarketAssignment, that constraint excludes on
// tenant_id alone (not tenant_id+profile_id): a tenant SHALL have at most
// one active isolation profile assignment at any given time, regardless of
// which profile.
type TenantIsolationProfileAssignment struct {
	TenantID           string     `json:"tenant_id"`
	IsolationProfileID string     `json:"isolation_profile_id"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveTo        *time.Time `json:"effective_to,omitempty"`
}

func (a TenantIsolationProfileAssignment) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(a.IsolationProfileID) == "" {
		return errors.New("isolation_profile_id is required")
	}
	if a.EffectiveFrom.IsZero() {
		return errors.New("effective_from is required")
	}
	if a.EffectiveTo != nil && !a.EffectiveTo.After(a.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	return nil
}
