package auth

import "errors"

var ErrTenantAccessDenied = errors.New("tenant access denied")

// TenantGrant is an explicit, least-privilege authorization for a principal
// to operate in a tenant other than the tenant bound to its verified token.
type TenantGrant struct {
	PrincipalID string
	TenantID    string
	Permission  string
	Active      bool
}

// AuthorizeTenantAccess enforces tenant isolation. Organization ownership or
// hierarchy is deliberately not an implicit grant to subsidiary operational
// state.
func AuthorizeTenantAccess(principal Principal, targetTenantID, permission string, grants []TenantGrant) error {
	if principal.Subject == "" || principal.TenantID == "" || targetTenantID == "" || permission == "" {
		return ErrTenantAccessDenied
	}
	if principal.TenantID == targetTenantID {
		return nil
	}
	for _, grant := range grants {
		if grant.Active && grant.PrincipalID == principal.Subject && grant.TenantID == targetTenantID && grant.Permission == permission {
			return nil
		}
	}
	return ErrTenantAccessDenied
}
