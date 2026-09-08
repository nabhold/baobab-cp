package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/nabhold/baobab-cp/internal/domain"
)

func TestCapabilityResolverResolveUsesHighestPriorityBinding(t *testing.T) {
	resolver := CapabilityResolverImpl{}
	query := CapabilityResolutionQuery{
		CapabilityKey: "baobab_trade",
		Context: Context{
			TenantID:      "tenant-123",
			LegalEntityID: "legal-456",
			MarketID:      "market-789",
			CountryCode:   "ZA",
			CurrencyCode:  "ZAR",
			Locale:        "en-ZA",
		},
		Bindings: []CapabilityBinding{
			{CapabilityKey: "baobab_trade", EngineID: "engine-1", EngineInstanceID: "instance-1", BindingMode: "SECONDARY", Status: "ACTIVE", Priority: 10, ContractVersion: "v1"},
			{CapabilityKey: "baobab_trade", EngineID: "engine-2", EngineInstanceID: "instance-2", BindingMode: "PRIMARY", Status: "ACTIVE", Priority: 100, ContractVersion: "v1"},
		},
	}

	resolved, err := resolver.Resolve(context.Background(), query)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.BindingMode != "PRIMARY" {
		t.Fatalf("expected PRIMARY binding, got %q", resolved.BindingMode)
	}
	if resolved.EngineID != "engine-2" {
		t.Fatalf("expected engine-2, got %q", resolved.EngineID)
	}
}

func TestCapabilityResolverUsesScopeBeforePriority(t *testing.T) {
	resolver := CapabilityResolverImpl{}
	query := CapabilityResolutionQuery{
		CapabilityKey: "erp.receivables",
		Context:       Context{TenantID: "tn_zuribeans", MarketID: "market-za"},
		Scopes: map[string]domain.MappingScope{
			"global": {},
			"tenant": {TenantID: "tn_zuribeans", MarketID: "market-za"},
		},
		Bindings: []CapabilityBinding{
			{ID: "global", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-global", ScopeID: "global", BindingMode: "PRIMARY", Priority: 1000, Status: "ACTIVE"},
			{ID: "tenant", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-za", ScopeID: "tenant", BindingMode: "PRIMARY", Priority: 10, Status: "ACTIVE"},
		},
	}
	resolved, err := resolver.Resolve(context.Background(), query)
	if err != nil {
		t.Fatalf("resolve scoped binding: %v", err)
	}
	if resolved.BindingID != "tenant" || resolved.Specificity != 2 {
		t.Fatalf("expected tenant/market binding, got %#v", resolved)
	}
}

func TestCapabilityResolverAppliesTemporalWindow(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	resolver := CapabilityResolverImpl{}
	resolved, err := resolver.Resolve(context.Background(), CapabilityResolutionQuery{
		CapabilityKey: "erp.receivables",
		Context:       Context{TenantID: "tn_zuribeans"},
		At:            now,
		Bindings: []CapabilityBinding{
			{ID: "expired", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-old", BindingMode: "PRIMARY", Priority: 100, Status: "ACTIVE", EffectiveFrom: past.Add(-time.Hour), EffectiveTo: &past},
			{ID: "current", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-current", BindingMode: "PRIMARY", Priority: 10, Status: "ACTIVE", EffectiveFrom: past, EffectiveTo: &future},
		},
	})
	if err != nil || resolved.BindingID != "current" {
		t.Fatalf("expected current temporal binding, got %#v err=%v", resolved, err)
	}
}

func TestCapabilityResolverFailsClosedOnAmbiguity(t *testing.T) {
	resolver := CapabilityResolverImpl{}
	_, err := resolver.Resolve(context.Background(), CapabilityResolutionQuery{
		CapabilityKey: "erp.receivables",
		Context:       Context{TenantID: "tn_zuribeans"},
		Bindings: []CapabilityBinding{
			{ID: "a", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-a", BindingMode: "PRIMARY", Priority: 100, Status: "ACTIVE"},
			{ID: "b", CapabilityKey: "erp.receivables", EngineInstanceID: "erp-b", BindingMode: "PRIMARY", Priority: 100, Status: "ACTIVE"},
		},
	})
	if err == nil || err.Error() != "capability binding is ambiguous" {
		t.Fatalf("expected ambiguity failure, got %v", err)
	}
}

func TestCapabilityResolverRejectsUnknownCapability(t *testing.T) {
	resolver := CapabilityResolverImpl{}
	_, err := resolver.Resolve(context.Background(), CapabilityResolutionQuery{
		CapabilityKey: "missing_capability",
		Context:       Context{TenantID: "tenant-123"},
		Bindings:      nil,
	})
	if err == nil {
		t.Fatal("expected missing capability error")
	}
}
