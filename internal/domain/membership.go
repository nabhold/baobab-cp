package domain

import "errors"

// WorkforceMembership is ADR-0009 §27's "Workforce Membership" relationship:
//
//	CanonicalIdentity
//	      │
//	      ▼
//	WorkforceMembership
//	      │
//	      ├── LegalEntity
//	      ├── Tenant
//	      └── status
//
// Deliberately distinct from a workforce human's `Principal` (ADR-0004):
// per §28 ("Employment Status Is Not IAM Credential Status") and §122's
// "Workforce Membership ≠ Canonical Identity" invariant, a Principal can
// outlive its workforce relationship (the same human may remain a
// customer/supplier contact after leaving), and a Principal is not itself
// scoped to any tenant. This is the join: which tenant(s) a given
// workforce Principal is actually a member of, with what role(s), and
// whether that membership is currently active -- the fact `cp:tenant-admin`
// realm-role tokens (Gate IAM-5 phase 1, baobab-iam) carry no tenant scope
// of their own is exactly why this table exists.
type WorkforceMembership struct {
	ID            string   `json:"id,omitempty"`
	PrincipalID   string   `json:"principal_id"`
	TenantID      string   `json:"tenant_id"`
	LegalEntityID string   `json:"legal_entity_id,omitempty"`
	Roles         []string `json:"roles"`
	Status        string   `json:"status"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

// validWorkforceMembershipStatuses intentionally mirrors Principal's own
// ACTIVE/SUSPENDED/DISABLED vocabulary (not a separate one) -- ADR-0009 §28
// keeps employment status and IAM credential status as two independently
// tracked fields, not two different vocabularies to reconcile.
var validWorkforceMembershipStatuses = map[string]bool{"ACTIVE": true, "SUSPENDED": true, "DISABLED": true}

// ValidWorkforceMembershipStatus reports whether status is one of this
// domain's recognized WorkforceMembership lifecycle states -- exported so
// SetWorkforceMembershipStatus implementations (in-memory and Postgres)
// can validate a bare status transition without duplicating the
// vocabulary.
func ValidWorkforceMembershipStatus(status string) bool {
	return validWorkforceMembershipStatuses[status]
}

func (m WorkforceMembership) Validate() error {
	if m.PrincipalID == "" {
		return errors.New("principal_id is required")
	}
	if !ValidTenantID(m.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if len(m.Roles) == 0 {
		return errors.New("at least one role is required")
	}
	for _, role := range m.Roles {
		if role == "" {
			return errors.New("role must not be empty")
		}
	}
	if !validWorkforceMembershipStatuses[m.Status] {
		return errors.New("status must be ACTIVE, SUSPENDED or DISABLED")
	}
	return nil
}

// HasRole reports whether this membership carries the given role, active or
// not -- callers that care about active status check m.Status separately
// (mirroring auth.Principal.HasScope's own plain membership-test shape).
func (m WorkforceMembership) HasRole(role string) bool {
	for _, r := range m.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// NewWorkforceMembershipID mints a new workforce-membership identifier,
// following the same bare-UUID convention as NewPrincipalID.
func NewWorkforceMembershipID() string { return NewUUIDv7() }
