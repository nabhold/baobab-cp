package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var marketCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Market models market.market (migration 000006) -- a market a tenant may
// participate in, per ADR-BCP-004 §4/§55. The table has existed since early
// in this codebase's history; this is the first Go code to read or write it.
type Market struct {
	ID       string `json:"id,omitempty"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Region   string `json:"region"`
	IsActive bool   `json:"is_active"`
}

func (m Market) Validate() error {
	if strings.TrimSpace(m.Code) == "" {
		return errors.New("code is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name is required")
	}
	if !marketCurrencyPattern.MatchString(m.Currency) {
		return errors.New("currency must be a 3-letter uppercase ISO code")
	}
	if strings.TrimSpace(m.Region) == "" {
		return errors.New("region is required")
	}
	return nil
}

// MarketAssignment models market.market_assignment (migration 000006,
// extended by migration 000024 with a generated valid_period column and a
// market_assignment_active_excl exclusion constraint). That constraint
// SHALL reject two assignments for the same (tenant_id, market_id) pair
// with overlapping [effective_from, effective_to) periods -- a tenant MAY
// have concurrent assignments to different markets, just not two
// overlapping assignments to the same one.
type MarketAssignment struct {
	ID            string     `json:"id,omitempty"`
	TenantID      string     `json:"tenant_id"`
	MarketID      string     `json:"market_id"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
}

func (a MarketAssignment) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(a.MarketID) == "" {
		return errors.New("market_id is required")
	}
	if a.EffectiveFrom.IsZero() {
		return errors.New("effective_from is required")
	}
	if a.EffectiveTo != nil && !a.EffectiveTo.After(a.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	return nil
}
