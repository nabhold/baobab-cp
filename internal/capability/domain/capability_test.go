package domain

import (
	"testing"
	"time"
)

func TestCapabilityKeyIsImplementationNeutral(t *testing.T) {
	for _, key := range []string{"erp.receivables", "warehouse.execution", "intelligence.supplier-research"} {
		if err := (Capability{Key: key, Name: key, Status: "ACTIVE"}).Validate(); err != nil {
			t.Fatalf("valid capability key %q rejected: %v", key, err)
		}
	}
	for _, key := range []string{"idempiere.C_Invoice", "haystack", "baobab_trade"} {
		if err := (Capability{Key: key, Name: key, Status: "ACTIVE"}).Validate(); err == nil {
			t.Fatalf("implementation-coupled or non-canonical capability key %q accepted", key)
		}
	}
}

func TestCapabilityBindingRejectsInvalidInterval(t *testing.T) {
	now := time.Now().UTC()
	end := now.Add(-time.Minute)
	binding := CapabilityBinding{
		CapabilityKey:    "erp.receivables",
		EngineInstanceID: "0199-erp-instance",
		ScopeID:          "0199-scope",
		EffectiveFrom:    now,
		EffectiveTo:      &end,
	}
	if err := binding.Validate(); err == nil {
		t.Fatal("reversed binding validity interval accepted")
	}
}

func TestCapabilityProviderRequiresOwningRepositoryDotEngineKey(t *testing.T) {
	valid := CapabilityProvider{ProviderKey: "baobab-trade.medusa", Name: "Medusa", ProviderType: "BAOBAB_ENGINE", EngineID: "engine-1"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid provider rejected: %v", err)
	}
	invalid := valid
	invalid.ProviderKey = "medusa"
	if err := invalid.Validate(); err == nil {
		t.Fatal("provider_key without <owning-repository>.<engine> form accepted")
	}
}

func TestBindingModeValid(t *testing.T) {
	for _, mode := range []BindingMode{BindingModePrimary, BindingModeFallback, BindingModeShadow, BindingModeMigration, BindingModeDisabled} {
		if !mode.Valid() {
			t.Fatalf("canonical binding mode %q rejected as invalid", mode)
		}
	}
	for _, mode := range []BindingMode{"SECONDARY", "READ_ONLY", "MIGRATION_SOURCE", "MIGRATION_TARGET", ""} {
		if mode.Valid() {
			t.Fatalf("superseded or empty binding mode %q accepted as valid", mode)
		}
	}
}
