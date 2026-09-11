package domain

import (
	"testing"
	"time"
)

func TestCapabilityGrantRequiresSourceReferenceUnlessPlatformBaseline(t *testing.T) {
	base := CapabilityGrant{
		TenantID:      "tn_zuribeans",
		CapabilityKey: "erp.receivables",
		ScopeID:       "scope-1",
		Status:        GrantStatusActive,
		EffectiveFrom: time.Now().UTC(),
	}

	baseline := base
	baseline.Source = GrantSourcePlatformBaseline
	if err := baseline.Validate(); err != nil {
		t.Fatalf("platform-baseline grant without source_reference rejected: %v", err)
	}

	subscription := base
	subscription.Source = GrantSourceProductSubscription
	if err := subscription.Validate(); err == nil {
		t.Fatal("non-baseline grant without source_reference accepted")
	}
	subscription.SourceReference = "subscription_123"
	if err := subscription.Validate(); err != nil {
		t.Fatalf("valid subscription grant rejected: %v", err)
	}
}

func TestCapabilityGrantRejectsInvalidSourceOrStatus(t *testing.T) {
	grant := CapabilityGrant{
		TenantID:        "tn_zuribeans",
		CapabilityKey:   "erp.receivables",
		ScopeID:         "scope-1",
		Source:          "VENDOR_DISCRETION",
		SourceReference: "x",
		Status:          GrantStatusActive,
		EffectiveFrom:   time.Now().UTC(),
	}
	if err := grant.Validate(); err == nil {
		t.Fatal("non-canonical grant source accepted")
	}

	grant.Source = GrantSourceManualApproval
	grant.Status = "APPROVED"
	if err := grant.Validate(); err == nil {
		t.Fatal("non-canonical grant status accepted")
	}
}

func TestCapabilityGrantRejectsInvertedEffectiveWindow(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	grant := CapabilityGrant{
		TenantID:      "tn_zuribeans",
		CapabilityKey: "erp.receivables",
		ScopeID:       "scope-1",
		Source:        GrantSourcePlatformBaseline,
		Status:        GrantStatusActive,
		EffectiveFrom: now,
		EffectiveTo:   &past,
	}
	if err := grant.Validate(); err == nil {
		t.Fatal("grant with effective_to before effective_from accepted")
	}
}

func TestCapabilityGrantIsEffective(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name  string
		grant CapabilityGrant
		want  bool
	}{
		{"active within window", CapabilityGrant{Status: GrantStatusActive, EffectiveFrom: past, EffectiveTo: &future}, true},
		{"active with no end", CapabilityGrant{Status: GrantStatusActive, EffectiveFrom: past}, true},
		{"pending", CapabilityGrant{Status: GrantStatusPending, EffectiveFrom: past}, false},
		{"suspended", CapabilityGrant{Status: GrantStatusSuspended, EffectiveFrom: past}, false},
		{"revoked", CapabilityGrant{Status: GrantStatusRevoked, EffectiveFrom: past}, false},
		{"expired status", CapabilityGrant{Status: GrantStatusExpired, EffectiveFrom: past}, false},
		{"not yet effective", CapabilityGrant{Status: GrantStatusActive, EffectiveFrom: future}, false},
		{"past its effective_to", CapabilityGrant{Status: GrantStatusActive, EffectiveFrom: past.Add(-time.Hour), EffectiveTo: &past}, false},
	}
	for _, tc := range cases {
		if got := tc.grant.IsEffective(now); got != tc.want {
			t.Errorf("%s: IsEffective() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// PLATFORM_BASELINE and an enterprise product grant SHALL be able to
// coexist for the same capability -- grant ambiguity (unlike binding
// ambiguity) is not an error; the resolver treats entitlement as
// satisfied when at least one effective compatible grant exists (§35).
// This is exercised at the resolver-integration level once grants are
// wired into CapabilityResolverImpl; this package only guarantees both
// grants independently validate and report their own effectiveness.
func TestMultipleGrantsForSameCapabilityAreIndependentlyValid(t *testing.T) {
	now := time.Now().UTC()
	baseline := CapabilityGrant{
		TenantID: "tn_zuribeans", CapabilityKey: "erp.receivables", ScopeID: "scope-1",
		Source: GrantSourcePlatformBaseline, Status: GrantStatusActive, EffectiveFrom: now.Add(-time.Hour),
	}
	subscription := CapabilityGrant{
		TenantID: "tn_zuribeans", CapabilityKey: "erp.receivables", ScopeID: "scope-1",
		Source: GrantSourceProductSubscription, SourceReference: "subscription_123",
		Status: GrantStatusActive, EffectiveFrom: now.Add(-time.Hour),
	}
	if err := baseline.Validate(); err != nil {
		t.Fatalf("baseline grant invalid: %v", err)
	}
	if err := subscription.Validate(); err != nil {
		t.Fatalf("subscription grant invalid: %v", err)
	}
	if !baseline.IsEffective(now) || !subscription.IsEffective(now) {
		t.Fatal("both coexisting grants should be independently effective")
	}
}
