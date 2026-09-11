package domain

import (
	"testing"
	"time"
)

func validCapability(key string) Capability {
	return Capability{Key: key, Name: key, DomainKey: "commerce", Lifecycle: CapabilityLifecycleActive, Maturity: CapabilityMaturitySupported}
}

func TestCapabilityKeyIsImplementationNeutral(t *testing.T) {
	for _, key := range []string{"erp.receivables", "warehouse.execution", "intelligence.supplier-research"} {
		if err := validCapability(key).Validate(); err != nil {
			t.Fatalf("valid capability key %q rejected: %v", key, err)
		}
	}
	for _, key := range []string{"idempiere.C_Invoice", "haystack", "baobab_trade"} {
		if err := validCapability(key).Validate(); err == nil {
			t.Fatalf("implementation-coupled or non-canonical capability key %q accepted", key)
		}
	}
}

func TestCapabilityRequiresDomainLifecycleAndMaturity(t *testing.T) {
	base := validCapability("erp.receivables")
	if err := base.Validate(); err != nil {
		t.Fatalf("valid capability rejected: %v", err)
	}

	noDomain := base
	noDomain.DomainKey = ""
	if err := noDomain.Validate(); err == nil {
		t.Fatal("capability without domain accepted")
	}

	badLifecycle := base
	badLifecycle.Lifecycle = "PUBLISHED"
	if err := badLifecycle.Validate(); err == nil {
		t.Fatal("non-canonical lifecycle value accepted")
	}

	badMaturity := base
	badMaturity.Maturity = "BETA"
	if err := badMaturity.Validate(); err == nil {
		t.Fatal("non-canonical maturity value accepted")
	}
}

func TestCapabilityIsResolvable(t *testing.T) {
	cases := []struct {
		lifecycle CapabilityLifecycle
		want      bool
	}{
		{CapabilityLifecycleActive, true},
		{CapabilityLifecycleDraft, false},
		{CapabilityLifecycleSuspended, false},
		{CapabilityLifecycleDeprecated, false},
		{CapabilityLifecycleRetired, false},
	}
	for _, tc := range cases {
		c := validCapability("erp.receivables")
		c.Lifecycle = tc.lifecycle
		if got := c.IsResolvable(); got != tc.want {
			t.Errorf("lifecycle %s: IsResolvable() = %v, want %v", tc.lifecycle, got, tc.want)
		}
	}
}

func TestCapabilityDependencyValidation(t *testing.T) {
	required := CapabilityDependency{CapabilityKey: "commerce.order.create", DependsOnCapability: "finance.invoice.issue", DependencyType: DependencyTypeRequired}
	if err := required.Validate(); err != nil {
		t.Fatalf("valid required dependency rejected: %v", err)
	}

	selfDependency := required
	selfDependency.DependsOnCapability = "commerce.order.create"
	if err := selfDependency.Validate(); err == nil {
		t.Fatal("self-dependency accepted")
	}

	conditionalWithoutCondition := CapabilityDependency{CapabilityKey: "commerce.order.create", DependsOnCapability: "finance.invoice.issue", DependencyType: DependencyTypeConditional}
	if err := conditionalWithoutCondition.Validate(); err == nil {
		t.Fatal("CONDITIONAL dependency without a condition accepted")
	}

	conditionalWithCondition := conditionalWithoutCondition
	conditionalWithCondition.Condition = "order.total > 0"
	if err := conditionalWithCondition.Validate(); err != nil {
		t.Fatalf("valid conditional dependency rejected: %v", err)
	}

	invalidType := CapabilityDependency{CapabilityKey: "commerce.order.create", DependsOnCapability: "finance.invoice.issue", DependencyType: "MANDATORY"}
	if err := invalidType.Validate(); err == nil {
		t.Fatal("non-canonical dependency_type accepted")
	}
}

func TestHasCapabilityDependencyCycleDetectsRequiredCyclesOnly(t *testing.T) {
	acyclic := []CapabilityDependency{
		{CapabilityKey: "a", DependsOnCapability: "b", DependencyType: DependencyTypeRequired},
		{CapabilityKey: "b", DependsOnCapability: "c", DependencyType: DependencyTypeRequired},
	}
	if HasCapabilityDependencyCycle(acyclic) {
		t.Fatal("acyclic required dependency graph reported as cyclic")
	}

	cyclic := []CapabilityDependency{
		{CapabilityKey: "a", DependsOnCapability: "b", DependencyType: DependencyTypeRequired},
		{CapabilityKey: "b", DependsOnCapability: "c", DependencyType: DependencyTypeRequired},
		{CapabilityKey: "c", DependsOnCapability: "a", DependencyType: DependencyTypeRequired},
	}
	if !HasCapabilityDependencyCycle(cyclic) {
		t.Fatal("cyclic required dependency graph not detected")
	}

	// An OPTIONAL edge completing the same cycle SHALL NOT trigger the
	// acyclic requirement -- SS8 scopes it to REQUIRED dependencies only.
	optionalCycle := []CapabilityDependency{
		{CapabilityKey: "a", DependsOnCapability: "b", DependencyType: DependencyTypeRequired},
		{CapabilityKey: "b", DependsOnCapability: "a", DependencyType: DependencyTypeOptional},
	}
	if HasCapabilityDependencyCycle(optionalCycle) {
		t.Fatal("a cycle completed only via an OPTIONAL edge was reported as an acyclic-requirement violation")
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
