package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/resolver"
)

func TestInMemoryRepositoryLoadsResolverData(t *testing.T) {
	repo := NewInMemoryRepository()
	repo.Mappings["tenant-123"] = []domain.Mapping{{
		ID:                      "mapping-tenant",
		MappingType:             "IDENTITY",
		TenantID:                "tenant-123",
		CanonicalEntityID:       "tenant-123",
		TargetCanonicalEntityID: "entity-tenant",
		ScopeID:                 "tenant-123",
		Direction:               "BIDIRECTIONAL",
		Cardinality:             "ONE_TO_ONE",
		Authority:               "baobab",
		Confidence:              "CONFIRMED",
		Status:                  "ACTIVE",
		ResolutionPriority:      50,
		EffectiveFrom:           "2025-01-01T00:00:00Z",
	}}
	repo.Bindings["baobab_trade"] = []resolver.CapabilityBinding{{
		CapabilityKey:    "baobab_trade",
		EngineID:         "engine-1",
		EngineInstanceID: "instance-1",
		BindingMode:      "PRIMARY",
		Priority:         100,
		Status:           "ACTIVE",
		ContractVersion:  "v1",
	}}
	repo.EngineInstances["engine-1"] = []resolver.EngineInstance{{
		ID:          "instance-1",
		EngineID:    "engine-1",
		Region:      "af-south-1",
		Environment: "production",
		Status:      "ACTIVE",
	}}

	mappings, err := repo.ListMappings(context.Background(), "tenant-123")
	if err != nil {
		t.Fatalf("list mappings failed: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected one mapping, got %d", len(mappings))
	}

	bindings, err := repo.ListBindings(context.Background(), "baobab_trade")
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one binding, got %d", len(bindings))
	}

	instances, err := repo.ListActiveInstances(context.Background(), "engine-1")
	if err != nil {
		t.Fatalf("list active instances failed: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected one active instance, got %d", len(instances))
	}
}

func TestInMemoryRepositoryPersistsMappingsAndBindingsWithVersions(t *testing.T) {
	repo := NewInMemoryRepository()
	mapping := domain.Mapping{
		ID: "mapping-1", MappingType: "IDENTITY", TenantID: "entity-1",
		CanonicalEntityID: "entity-1", TargetCanonicalEntityID: "entity-2", ScopeID: "scope-1",
		Direction: "BIDIRECTIONAL", Cardinality: "ONE_TO_ONE", Authority: "baobab",
		Confidence: "CONFIRMED", Status: "ACTIVE", EffectiveFrom: "2025-01-01T00:00:00Z",
	}
	if err := repo.CreateMapping(context.Background(), mapping); err != nil {
		t.Fatalf("create mapping failed: %v", err)
	}
	mapping.Status = "SUSPENDED"
	if err := repo.SaveMapping(context.Background(), mapping, 1); err != nil {
		t.Fatalf("save mapping failed: %v", err)
	}
	savedMapping, err := repo.GetMapping(context.Background(), "mapping-1")
	if err != nil {
		t.Fatalf("get mapping failed: %v", err)
	}
	if savedMapping.Revision != 2 || savedMapping.Status != "SUSPENDED" {
		t.Fatalf("unexpected saved mapping: %+v", savedMapping)
	}
	if err := repo.SaveMapping(context.Background(), mapping, 1); err == nil {
		t.Fatal("expected mapping version conflict")
	}

	binding := resolver.CapabilityBinding{
		CapabilityKey: "trade", EngineID: "engine-1", EngineInstanceID: "instance-1",
		BindingMode: "PRIMARY", Status: "ACTIVE", ContractVersion: "v1",
	}
	if err := repo.CreateBinding(context.Background(), binding); err != nil {
		t.Fatalf("create binding failed: %v", err)
	}
	binding.Status = "SUSPENDED"
	if err := repo.SaveBinding(context.Background(), binding, 1); err != nil {
		t.Fatalf("save binding failed: %v", err)
	}
	bindings, err := repo.ListBindings(context.Background(), "trade")
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Version != 2 || bindings[0].Status != "SUSPENDED" {
		t.Fatalf("unexpected saved bindings: %+v", bindings)
	}
}

func TestInMemoryRepositoryPersistsMappingScopes(t *testing.T) {
	repo := NewInMemoryRepository()
	scope := domain.MappingScope{
		ScopeID:  "scope-1",
		TenantID: "tenant-1",
		Country:  "KE",
		Currency: "KES",
	}
	if err := repo.CreateMappingScope(context.Background(), scope); err != nil {
		t.Fatalf("create mapping scope failed: %v", err)
	}
	if err := repo.CreateMappingScope(context.Background(), scope); err == nil {
		t.Fatal("expected duplicate mapping scope to be rejected")
	}

	empty := domain.MappingScope{ScopeID: "scope-invalid"}
	if err := repo.CreateMappingScope(context.Background(), empty); err == nil {
		t.Fatal("expected mapping scope without tenant_id to be rejected")
	}

	fetched, err := repo.GetMappingScope(context.Background(), "scope-1")
	if err != nil {
		t.Fatalf("get mapping scope failed: %v", err)
	}
	if fetched.Country != "KE" || fetched.Currency != "KES" {
		t.Fatalf("unexpected fetched mapping scope: %+v", fetched)
	}

	if _, err := repo.GetMappingScope(context.Background(), "missing"); err == nil {
		t.Fatal("expected missing mapping scope lookup to fail")
	}

	other := domain.MappingScope{ScopeID: "scope-2", TenantID: "tenant-2"}
	if err := repo.CreateMappingScope(context.Background(), other); err != nil {
		t.Fatalf("create second mapping scope failed: %v", err)
	}

	scopes, err := repo.ListMappingScopes(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("list mapping scopes failed: %v", err)
	}
	if len(scopes) != 1 || scopes[0].ScopeID != "scope-1" {
		t.Fatalf("unexpected mapping scopes for tenant-1: %+v", scopes)
	}
}

// TestInMemoryRepositoryResolvesAndProvisionsIdentity exercises Gate IAM-3
// phase 2's IdentityRepository against ADR-0004 §11's resolution flow: an
// unknown (issuer, subject) pair returns ErrIdentityNotFound (the "absent"
// branch a phase-3 provisioning flow would react to), and once a Principal
// is created and an ExternalIdentity linked to it, the same pair resolves
// to that Principal.
func TestInMemoryRepositoryResolvesAndProvisionsIdentity(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	issuer, subject := "https://iam.nabhold.com/realms/baobab", "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	if _, err := repo.ResolveIdentity(ctx, issuer, subject); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected ErrIdentityNotFound for unknown (issuer, subject), got %v", err)
	}

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if err := repo.CreateIdentity(ctx, principal); err == nil {
		t.Fatal("expected duplicate principal id to be rejected")
	}

	external := domain.ExternalIdentity{
		ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID,
		Issuer: issuer, Subject: subject, ProviderType: "keycloak", Status: "ACTIVE",
	}
	if err := repo.LinkExternalIdentity(ctx, external); err != nil {
		t.Fatalf("link external identity failed: %v", err)
	}
	if err := repo.LinkExternalIdentity(ctx, external); err == nil {
		t.Fatal("expected duplicate (issuer, subject) to be rejected")
	}

	resolved, err := repo.ResolveIdentity(ctx, issuer, subject)
	if err != nil {
		t.Fatalf("resolve identity failed: %v", err)
	}
	if resolved.ID != principal.ID || resolved.ActorType != "human" || resolved.Status != "ACTIVE" {
		t.Fatalf("unexpected resolved principal: %+v", resolved)
	}

	orphan := domain.ExternalIdentity{
		ID: domain.NewExternalIdentityID(), PrincipalID: "does-not-exist",
		Issuer: issuer, Subject: "other-subject", Status: "ACTIVE",
	}
	if err := repo.LinkExternalIdentity(ctx, orphan); err == nil {
		t.Fatal("expected linking to a nonexistent principal to be rejected")
	}
}

// TestInMemoryRepositoryMapsAndResolvesIdentityReferences exercises Gate
// IAM-3 phase 5's IdentityReferenceRepository against ADR-0004 §23-29: an
// unknown engine-native actor returns ErrIdentityReferenceNotFound, mapping
// one to a Principal makes it resolvable, a second Principal cannot claim
// the same engine-native actor (§29's collision-avoidance invariant), and
// ListIdentityReferences returns every mapping for a given Principal.
func TestInMemoryRepositoryMapsAndResolvesIdentityReferences(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := repo.ResolveIdentityReference(ctx, "baobab-trade", "customer", "C-100"); !errors.Is(err, ErrIdentityReferenceNotFound) {
		t.Fatalf("expected ErrIdentityReferenceNotFound for an unmapped engine-native actor, got %v", err)
	}

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}

	tradeReference := domain.IdentityReference{
		ID: domain.NewIdentityReferenceID(), PrincipalID: principal.ID,
		Engine: "baobab-trade", ExternalType: "customer", ExternalID: "C-100", Status: "ACTIVE",
	}
	if err := repo.CreateIdentityReference(ctx, tradeReference); err != nil {
		t.Fatalf("create identity reference failed: %v", err)
	}

	other := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, other); err != nil {
		t.Fatalf("create second identity failed: %v", err)
	}
	conflicting := domain.IdentityReference{
		ID: domain.NewIdentityReferenceID(), PrincipalID: other.ID,
		Engine: "baobab-trade", ExternalType: "customer", ExternalID: "C-100", Status: "ACTIVE",
	}
	if err := repo.CreateIdentityReference(ctx, conflicting); !errors.Is(err, ErrIdentityReferenceAlreadyMapped) {
		t.Fatalf("expected ErrIdentityReferenceAlreadyMapped for a duplicate engine-native actor, got %v", err)
	}

	resolved, err := repo.ResolveIdentityReference(ctx, "baobab-trade", "customer", "C-100")
	if err != nil {
		t.Fatalf("resolve identity reference failed: %v", err)
	}
	if resolved.PrincipalID != principal.ID {
		t.Fatalf("expected reference to resolve to principal %s, got %s", principal.ID, resolved.PrincipalID)
	}

	// ADR-0004 §28: one Principal may have multiple engine mappings.
	erpReference := domain.IdentityReference{
		ID: domain.NewIdentityReferenceID(), PrincipalID: principal.ID,
		Engine: "baobab-erp", ExternalType: "ad_user", ExternalID: "20013", Status: "ACTIVE",
	}
	if err := repo.CreateIdentityReference(ctx, erpReference); err != nil {
		t.Fatalf("create second identity reference failed: %v", err)
	}
	references, err := repo.ListIdentityReferences(ctx, principal.ID)
	if err != nil {
		t.Fatalf("list identity references failed: %v", err)
	}
	if len(references) != 2 {
		t.Fatalf("expected 2 identity references for principal %s, got %d", principal.ID, len(references))
	}

	orphan := domain.IdentityReference{
		ID: domain.NewIdentityReferenceID(), PrincipalID: "does-not-exist",
		Engine: "baobab-cms", ExternalType: "user", ExternalID: "P-77", Status: "ACTIVE",
	}
	if err := repo.CreateIdentityReference(ctx, orphan); err == nil {
		t.Fatal("expected mapping a nonexistent principal to be rejected")
	}
}

// TestInMemoryRepositoryGetPrincipal exercises Gate IAM-3 phase 6's
// principal-by-ID lookup, which ADR-0004 §15-22's linking/unlinking/merging
// flows all depend on (unlike ResolveIdentity's (issuer, subject) lookup).
func TestInMemoryRepositoryGetPrincipal(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	if _, err := repo.GetPrincipal(ctx, "does-not-exist"); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected ErrIdentityNotFound for an unknown principal id, got %v", err)
	}

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	fetched, err := repo.GetPrincipal(ctx, principal.ID)
	if err != nil {
		t.Fatalf("get principal failed: %v", err)
	}
	if fetched.ID != principal.ID || fetched.ActorType != "human" || fetched.Status != "ACTIVE" {
		t.Fatalf("unexpected fetched principal: %+v", fetched)
	}
}

// TestInMemoryRepositoryLinkExternalIdentityAudited exercises Gate IAM-3
// phase 6's audited linking flow (ADR-0004 §16, "Every successful link
// SHALL be auditable"): a successful link is recorded in LinkAudit
// alongside the actual link, and a failed link (duplicate issuer/subject)
// records neither.
func TestInMemoryRepositoryLinkExternalIdentityAudited(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}

	external := domain.ExternalIdentity{
		ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID,
		Issuer: "https://accounts.google.com", Subject: "google-sub-1", ProviderType: "google", Status: "ACTIVE",
	}
	actor := AuditActor{ActorID: principal.ID, ActorType: "human", CorrelationID: "11111111-1111-4111-8111-111111111111"}
	if err := repo.LinkExternalIdentityAudited(ctx, external, actor, "user requested additional login method"); err != nil {
		t.Fatalf("link external identity audited failed: %v", err)
	}
	if len(repo.LinkAudit) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(repo.LinkAudit))
	}
	if repo.LinkAudit[0].External.ID != external.ID || repo.LinkAudit[0].Actor.ActorID != principal.ID {
		t.Fatalf("unexpected audit record: %+v", repo.LinkAudit[0])
	}

	if err := repo.LinkExternalIdentityAudited(ctx, external, actor, "duplicate attempt"); err == nil {
		t.Fatal("expected duplicate (issuer, subject) to be rejected")
	}
	if len(repo.LinkAudit) != 1 {
		t.Fatalf("expected no additional audit record on a failed link, got %d total", len(repo.LinkAudit))
	}
}
