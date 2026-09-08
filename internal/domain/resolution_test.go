package domain

import (
	"testing"
	"time"
)

func TestContextRequiresTrustedAuthoritativeIdentity(t *testing.T) {
	now := time.Now().UTC()
	valid := Context{
		PrincipalID:   "principal:workload:zuribeans",
		TenantID:      "tn_zuribeans",
		CorrelationID: "01K4N5M6P7Q8R9S0T1V2W3X4Y5",
		ResolvedAt:    now,
		Provenance: map[string]ContextSource{
			"tenant_id": {Source: "verified_token", TrustLevel: TrustVerified},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Context rejected: %v", err)
	}

	spoofed := valid
	spoofed.Provenance = map[string]ContextSource{
		"tenant_id": {Source: "request_header", TrustLevel: TrustUntrusted},
	}
	if err := spoofed.Validate(); err == nil {
		t.Fatal("untrusted tenant provenance must fail closed")
	}
}

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

func TestExternalReferenceRequiresCanonicalAndNativeIdentity(t *testing.T) {
	reference := ExternalReference{
		CanonicalEntityID: "0199-canonical-party",
		EngineID:          "0199-idempiere",
		NativeType:        "C_BPartner",
		NativeID:          "10043",
		Status:            "ACTIVE",
	}
	if err := reference.Validate(); err != nil {
		t.Fatalf("valid external reference rejected: %v", err)
	}
	reference.NativeID = ""
	if err := reference.Validate(); err == nil {
		t.Fatal("external reference without native identity accepted")
	}
}
