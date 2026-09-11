package domain

import (
	"errors"
	"time"
)

type ResolveContext struct {
	ProductID string `json:"product_id"`
	// TenantID is optional: a workload token that already carries its own
	// tenant_id claim needs no request-supplied value (it must equal the
	// claim if present, per api.resolveWorkloadTenant); a token with none
	// -- every real workload client today, see that function's doc comment
	// -- requires this field to say which tenant to resolve for.
	TenantID string `json:"tenant_id,omitempty"`
}

func (c ResolveContext) Validate() error {
	if !ValidProductID(c.ProductID) {
		return errors.New("product_id must be a canonical product identifier")
	}
	return nil
}

type ResolvedContext struct {
	TenantID        string    `json:"tenant_id"`
	EntityID        string    `json:"entity_id"`
	LifecycleStatus string    `json:"lifecycle_status"`
	ProductID       string    `json:"product_id"`
	Entitled        bool      `json:"entitled"`
	EntitlementTier *string   `json:"entitlement_tier,omitempty"`
	CacheTTLSeconds int       `json:"cache_ttl_seconds"`
	ResolvedAt      time.Time `json:"resolved_at"`
	CorrelationID   string    `json:"correlation_id"`
}
