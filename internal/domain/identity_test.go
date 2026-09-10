package domain

import "testing"

func TestPrincipalValidateRejectsUnknownActorType(t *testing.T) {
	p := Principal{ActorType: "admin", Status: "ACTIVE"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected unknown actor_type to be rejected")
	}
	for _, actorType := range []string{"human", "workload", "external"} {
		if err := (Principal{ActorType: actorType, Status: "ACTIVE"}).Validate(); err != nil {
			t.Fatalf("valid actor_type %q rejected: %v", actorType, err)
		}
	}
}

func TestPrincipalValidateRejectsUnknownStatus(t *testing.T) {
	p := Principal{ActorType: "human", Status: "active"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected lowercase status to be rejected (this codebase's lifecycle vocabulary is uppercase)")
	}
	for _, status := range []string{"ACTIVE", "SUSPENDED", "DISABLED", "ARCHIVED"} {
		if err := (Principal{ActorType: "human", Status: status}).Validate(); err != nil {
			t.Fatalf("valid status %q rejected: %v", status, err)
		}
	}
}

func TestNewPrincipalIDIsUniqueAndUUIDShaped(t *testing.T) {
	a, b := NewPrincipalID(), NewPrincipalID()
	if a == b {
		t.Fatal("expected distinct principal IDs")
	}
	if len(a) != 36 {
		t.Fatalf("expected a UUID-shaped (36-char) principal ID, got %q", a)
	}
}

func TestExternalIdentityValidateRequiresPrincipalIssuerAndSubject(t *testing.T) {
	cases := []struct {
		name string
		e    ExternalIdentity
	}{
		{"missing principal_id", ExternalIdentity{Issuer: "https://iam.nabhold.com", Subject: "sub-1", Status: "ACTIVE"}},
		{"missing issuer", ExternalIdentity{PrincipalID: "p-1", Subject: "sub-1", Status: "ACTIVE"}},
		{"missing subject", ExternalIdentity{PrincipalID: "p-1", Issuer: "https://iam.nabhold.com", Status: "ACTIVE"}},
		{"invalid status", ExternalIdentity{PrincipalID: "p-1", Issuer: "https://iam.nabhold.com", Subject: "sub-1", Status: "PENDING"}},
	}
	for _, c := range cases {
		if err := c.e.Validate(); err == nil {
			t.Fatalf("%s: expected rejection", c.name)
		}
	}

	valid := ExternalIdentity{PrincipalID: "p-1", Issuer: "https://iam.nabhold.com", Subject: "sub-1", Status: "ACTIVE"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid external identity rejected: %v", err)
	}
}
