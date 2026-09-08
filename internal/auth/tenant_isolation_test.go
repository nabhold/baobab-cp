package auth

import (
	"errors"
	"testing"
)

func TestTenantIsolationBlocksSubsidiariesBidirectionally(t *testing.T) {
	tests := []struct {
		name      string
		principal Principal
		target    string
	}{
		{name: "Zuribeans cannot access Thamani", principal: Principal{Subject: "zuribeans-service", TenantID: "tn_zuribeans"}, target: "tn_thamani"},
		{name: "Thamani cannot access Zuribeans", principal: Principal{Subject: "thamani-service", TenantID: "tn_thamani"}, target: "tn_zuribeans"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AuthorizeTenantAccess(tt.principal, tt.target, "operational:read", nil); !errors.Is(err, ErrTenantAccessDenied) {
				t.Fatalf("expected tenant access denial, got %v", err)
			}
		})
	}
}

func TestParentOrganizationHasNoImplicitSubsidiaryAccess(t *testing.T) {
	principal := Principal{Subject: "nabhold-operator", TenantID: "tn_nabhold"}
	if err := AuthorizeTenantAccess(principal, "tn_zuribeans", "operational:read", nil); !errors.Is(err, ErrTenantAccessDenied) {
		t.Fatalf("expected parent organization denial, got %v", err)
	}
}

func TestExplicitTenantGrantAllowsOnlyNamedPermission(t *testing.T) {
	principal := Principal{Subject: "nabhold-operator", TenantID: "tn_nabhold"}
	grants := []TenantGrant{{
		PrincipalID: principal.Subject,
		TenantID:    "tn_zuribeans",
		Permission:  "operational:read",
		Active:      true,
	}}
	if err := AuthorizeTenantAccess(principal, "tn_zuribeans", "operational:read", grants); err != nil {
		t.Fatalf("expected explicit grant to authorize: %v", err)
	}
	if err := AuthorizeTenantAccess(principal, "tn_zuribeans", "operational:write", grants); !errors.Is(err, ErrTenantAccessDenied) {
		t.Fatalf("grant must not authorize another permission, got %v", err)
	}
	if err := AuthorizeTenantAccess(principal, "tn_thamani", "operational:read", grants); !errors.Is(err, ErrTenantAccessDenied) {
		t.Fatalf("grant must not authorize another tenant, got %v", err)
	}
}
