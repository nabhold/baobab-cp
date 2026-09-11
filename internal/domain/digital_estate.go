package domain

import (
	"errors"
	"strings"
	"time"
)

// DigitalEstateStatus mirrors estate.digital_estate's digital_estate_status_ck
// constraint (migration 000024), added after the table's own creation
// (migration 000007) and normalized to uppercase.
type DigitalEstateStatus string

const (
	DigitalEstateDraft     DigitalEstateStatus = "DRAFT"
	DigitalEstateActive    DigitalEstateStatus = "ACTIVE"
	DigitalEstateSuspended DigitalEstateStatus = "SUSPENDED"
	DigitalEstateRetired   DigitalEstateStatus = "RETIRED"
)

func (s DigitalEstateStatus) Valid() bool {
	switch s {
	case DigitalEstateDraft, DigitalEstateActive, DigitalEstateSuspended, DigitalEstateRetired:
		return true
	default:
		return false
	}
}

// DigitalEstate models estate.digital_estate (migration 000007) -- a
// tenant's digital estate per ADR-BCP-004 §4/§55 ("Tenant -> Digital
// Estates -> Digital Properties"). The table has existed since early in
// this codebase's history; this is the first Go code to read or write it.
//
// Domain is globally unique across the whole platform (the table's own
// UNIQUE(domain) constraint), not merely unique per tenant -- a digital
// estate's domain (e.g. "shop.zuribeans.com") identifies exactly one
// estate platform-wide.
type DigitalEstate struct {
	ID       string `json:"id,omitempty"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	// Status is optional on input: an empty value lets the database apply
	// its own default (ACTIVE) rather than every caller having to spell it
	// out.
	Status    DigitalEstateStatus `json:"status,omitempty"`
	CreatedAt time.Time           `json:"created_at,omitempty"`
}

func (e DigitalEstate) Validate() error {
	if strings.TrimSpace(e.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(e.Domain) == "" {
		return errors.New("domain is required")
	}
	if e.Status != "" && !e.Status.Valid() {
		return errors.New("status is invalid")
	}
	return nil
}
