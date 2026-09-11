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
