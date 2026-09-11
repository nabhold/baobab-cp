package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	capabilitydomain "github.com/nabhold/baobab-cp/internal/capability/domain"
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

// TestInMemoryRepositoryUnlinkExternalIdentityAudited exercises Gate IAM-3
// phase 6's audited unlinking flow (ADR-0004 §18): unlinking one of two
// ACTIVE credentials marks it UNLINKED (never deleted), records an audit
// entry, and the unlinked credential no longer resolves while the
// remaining one still does.
func TestInMemoryRepositoryUnlinkExternalIdentityAudited(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	primary := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID, Issuer: "https://iam.nabhold.com/realms/baobab", Subject: "keycloak-sub-1", Status: "ACTIVE"}
	secondary := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID, Issuer: "https://accounts.google.com", Subject: "google-sub-1", Status: "ACTIVE"}
	if err := repo.LinkExternalIdentity(ctx, primary); err != nil {
		t.Fatalf("link primary failed: %v", err)
	}
	if err := repo.LinkExternalIdentity(ctx, secondary); err != nil {
		t.Fatalf("link secondary failed: %v", err)
	}

	actor := AuditActor{ActorID: principal.ID, ActorType: "human"}
	if err := repo.UnlinkExternalIdentityAudited(ctx, principal.ID, secondary.Issuer, secondary.Subject, false, actor, "user removed a login method"); err != nil {
		t.Fatalf("unlink external identity audited failed: %v", err)
	}
	if len(repo.UnlinkAudit) != 1 || repo.UnlinkAudit[0].Administrative {
		t.Fatalf("unexpected unlink audit trail: %+v", repo.UnlinkAudit)
	}

	if _, err := repo.ResolveIdentity(ctx, secondary.Issuer, secondary.Subject); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected the unlinked identity to no longer resolve, got %v", err)
	}
	resolved, err := repo.ResolveIdentity(ctx, primary.Issuer, primary.Subject)
	if err != nil || resolved.ID != principal.ID {
		t.Fatalf("expected the remaining credential to still resolve, got principal=%+v err=%v", resolved, err)
	}
}

// TestInMemoryRepositoryUnlinkDeniesLastCredential covers ADR-0004 §18's
// core guard: unlinking a Principal's only ACTIVE credential is denied
// unless the operation is administrative.
func TestInMemoryRepositoryUnlinkDeniesLastCredential(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	only := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID, Issuer: "https://iam.nabhold.com/realms/baobab", Subject: "keycloak-sub-only", Status: "ACTIVE"}
	if err := repo.LinkExternalIdentity(ctx, only); err != nil {
		t.Fatalf("link failed: %v", err)
	}

	actor := AuditActor{ActorID: principal.ID, ActorType: "human"}
	if err := repo.UnlinkExternalIdentityAudited(ctx, principal.ID, only.Issuer, only.Subject, false, actor, "attempted self-unlink"); !errors.Is(err, ErrLastCredentialDenied) {
		t.Fatalf("expected ErrLastCredentialDenied, got %v", err)
	}
	if len(repo.UnlinkAudit) != 0 {
		t.Fatalf("expected no audit record for a denied unlink, got %d", len(repo.UnlinkAudit))
	}
	resolved, err := repo.ResolveIdentity(ctx, only.Issuer, only.Subject)
	if err != nil || resolved.ID != principal.ID {
		t.Fatalf("expected the last credential to remain linked, got principal=%+v err=%v", resolved, err)
	}

	// Administrative unlink bypasses the guard (§18: "administrative
	// recovery; explicit account disablement").
	if err := repo.UnlinkExternalIdentityAudited(ctx, principal.ID, only.Issuer, only.Subject, true, actor, "administrative account disablement"); err != nil {
		t.Fatalf("expected administrative unlink of the last credential to succeed, got %v", err)
	}
	if len(repo.UnlinkAudit) != 1 || !repo.UnlinkAudit[0].Administrative {
		t.Fatalf("unexpected unlink audit trail: %+v", repo.UnlinkAudit)
	}
}

func TestInMemoryRepositoryUnlinkRejectsMismatchOrUnknown(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	other := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if err := repo.CreateIdentity(ctx, other); err != nil {
		t.Fatalf("create second identity failed: %v", err)
	}
	linked := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: principal.ID, Issuer: "https://iam.nabhold.com/realms/baobab", Subject: "keycloak-sub-2", Status: "ACTIVE"}
	if err := repo.LinkExternalIdentity(ctx, linked); err != nil {
		t.Fatalf("link failed: %v", err)
	}

	actor := AuditActor{ActorID: other.ID, ActorType: "human"}
	if err := repo.UnlinkExternalIdentityAudited(ctx, other.ID, linked.Issuer, linked.Subject, false, actor, "wrong principal"); !errors.Is(err, ErrExternalIdentityNotLinked) {
		t.Fatalf("expected ErrExternalIdentityNotLinked for a mismatched principal, got %v", err)
	}
	if err := repo.UnlinkExternalIdentityAudited(ctx, principal.ID, "https://unknown.example", "no-such-subject", false, actor, "unknown identity"); !errors.Is(err, ErrExternalIdentityNotLinked) {
		t.Fatalf("expected ErrExternalIdentityNotLinked for an unknown external identity, got %v", err)
	}
}

// TestInMemoryRepositoryMergePrincipalsAudited exercises Gate IAM-3 phase
// 6's audited merge flow (ADR-0004 §19-21): every ExternalIdentity and
// IdentityReference belonging to the source Principal transfers to the
// target, the source is archived (never deleted), the target keeps its own
// pre-existing credentials, and the merge is recorded in MergeAudit.
func TestInMemoryRepositoryMergePrincipalsAudited(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	source := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	target := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, source); err != nil {
		t.Fatalf("create source failed: %v", err)
	}
	if err := repo.CreateIdentity(ctx, target); err != nil {
		t.Fatalf("create target failed: %v", err)
	}

	sourceExternal := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: source.ID, Issuer: "https://accounts.google.com", Subject: "google-sub-merge", Status: "ACTIVE"}
	if err := repo.LinkExternalIdentity(ctx, sourceExternal); err != nil {
		t.Fatalf("link source external identity failed: %v", err)
	}
	sourceReference := domain.IdentityReference{ID: domain.NewIdentityReferenceID(), PrincipalID: source.ID, Engine: "baobab-trade", ExternalType: "customer", ExternalID: "C-merge", Status: "ACTIVE"}
	if err := repo.CreateIdentityReference(ctx, sourceReference); err != nil {
		t.Fatalf("create source identity reference failed: %v", err)
	}
	targetExternal := domain.ExternalIdentity{ID: domain.NewExternalIdentityID(), PrincipalID: target.ID, Issuer: "https://iam.nabhold.com/realms/baobab", Subject: "keycloak-sub-merge", Status: "ACTIVE"}
	if err := repo.LinkExternalIdentity(ctx, targetExternal); err != nil {
		t.Fatalf("link target external identity failed: %v", err)
	}

	actor := AuditActor{ActorID: "admin-1", ActorType: "human"}
	if err := repo.MergePrincipalsAudited(ctx, source.ID, target.ID, actor, "duplicate accounts for the same person"); err != nil {
		t.Fatalf("merge principals audited failed: %v", err)
	}

	archivedSource, err := repo.GetPrincipal(ctx, source.ID)
	if err != nil {
		t.Fatalf("expected the archived source to remain resolvable, got %v", err)
	}
	if archivedSource.Status != "ARCHIVED" {
		t.Fatalf("expected source status ARCHIVED, got %q", archivedSource.Status)
	}

	resolved, err := repo.ResolveIdentity(ctx, sourceExternal.Issuer, sourceExternal.Subject)
	if err != nil || resolved.ID != target.ID {
		t.Fatalf("expected the transferred external identity to resolve to the target, got principal=%+v err=%v", resolved, err)
	}
	resolved, err = repo.ResolveIdentity(ctx, targetExternal.Issuer, targetExternal.Subject)
	if err != nil || resolved.ID != target.ID {
		t.Fatalf("expected the target's own pre-existing external identity to still resolve to the target, got principal=%+v err=%v", resolved, err)
	}

	transferredRef, err := repo.ResolveIdentityReference(ctx, sourceReference.Engine, sourceReference.ExternalType, sourceReference.ExternalID)
	if err != nil || transferredRef.PrincipalID != target.ID {
		t.Fatalf("expected the transferred identity reference to now belong to the target, got %+v err=%v", transferredRef, err)
	}

	if len(repo.MergeAudit) != 1 {
		t.Fatalf("expected 1 merge audit record, got %d", len(repo.MergeAudit))
	}
	record := repo.MergeAudit[0]
	if record.SourcePrincipalID != source.ID || record.TargetPrincipalID != target.ID {
		t.Fatalf("unexpected merge audit record: %+v", record)
	}
	if len(record.TransferredExternalIdentities) != 1 || len(record.TransferredIdentityReferences) != 1 {
		t.Fatalf("expected exactly the source's own rows to be recorded as transferred, got %+v", record)
	}
}

func TestInMemoryRepositoryMergeRejectsSameSourceAndTarget(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if err := repo.MergePrincipalsAudited(ctx, principal.ID, principal.ID, AuditActor{}, "self-merge"); !errors.Is(err, ErrMergeSourceEqualsTarget) {
		t.Fatalf("expected ErrMergeSourceEqualsTarget, got %v", err)
	}
}

func TestInMemoryRepositoryMergeRejectsUnknownPrincipal(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	principal := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if err := repo.MergePrincipalsAudited(ctx, "does-not-exist", principal.ID, AuditActor{}, "unknown source"); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected ErrIdentityNotFound for an unknown source, got %v", err)
	}
	if err := repo.MergePrincipalsAudited(ctx, principal.ID, "does-not-exist", AuditActor{}, "unknown target"); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("expected ErrIdentityNotFound for an unknown target, got %v", err)
	}
}

func TestInMemoryRepositoryMergeRejectsAlreadyArchivedPrincipal(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	a := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	b := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	c := domain.Principal{ID: domain.NewPrincipalID(), ActorType: "human", Status: "ACTIVE"}
	for _, p := range []domain.Principal{a, b, c} {
		if err := repo.CreateIdentity(ctx, p); err != nil {
			t.Fatalf("create identity failed: %v", err)
		}
	}
	actor := AuditActor{ActorID: "admin-1", ActorType: "human"}
	if err := repo.MergePrincipalsAudited(ctx, a.ID, b.ID, actor, "first merge"); err != nil {
		t.Fatalf("first merge failed: %v", err)
	}
	// a is now ARCHIVED -- it cannot be a source again, nor a target.
	if err := repo.MergePrincipalsAudited(ctx, a.ID, c.ID, actor, "archived source"); !errors.Is(err, ErrMergeNotEligible) {
		t.Fatalf("expected ErrMergeNotEligible for an archived source, got %v", err)
	}
	if err := repo.MergePrincipalsAudited(ctx, c.ID, a.ID, actor, "archived target"); !errors.Is(err, ErrMergeNotEligible) {
		t.Fatalf("expected ErrMergeNotEligible for an archived target, got %v", err)
	}
}

func TestInMemoryRepositoryCapabilityRegistry(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	capA := capabilitydomain.Capability{Key: "commerce.order.create", Name: "Order Create", DomainKey: "commerce", Lifecycle: capabilitydomain.CapabilityLifecycleActive, Maturity: capabilitydomain.CapabilityMaturitySupported}
	capB := capabilitydomain.Capability{Key: "finance.invoice.issue", Name: "Invoice Issue", DomainKey: "finance", Lifecycle: capabilitydomain.CapabilityLifecycleActive, Maturity: capabilitydomain.CapabilityMaturitySupported}
	if err := repo.CreateCapability(ctx, capA); err != nil {
		t.Fatalf("create capability failed: %v", err)
	}
	if err := repo.CreateCapability(ctx, capA); err == nil {
		t.Fatal("expected duplicate capability to be rejected")
	}
	if err := repo.CreateCapability(ctx, capB); err != nil {
		t.Fatalf("create second capability failed: %v", err)
	}

	fetched, err := repo.GetCapability(ctx, "commerce.order.create")
	if err != nil {
		t.Fatalf("get capability failed: %v", err)
	}
	if fetched.DomainKey != "commerce" || !fetched.IsResolvable() {
		t.Fatalf("unexpected fetched capability: %+v", fetched)
	}
	if _, err := repo.GetCapability(ctx, "missing.capability"); err == nil {
		t.Fatal("expected missing capability lookup to fail")
	}

	required := capabilitydomain.CapabilityDependency{CapabilityKey: "commerce.order.create", DependsOnCapability: "finance.invoice.issue", DependencyType: capabilitydomain.DependencyTypeRequired}
	if err := repo.CreateCapabilityDependency(ctx, required); err != nil {
		t.Fatalf("create dependency failed: %v", err)
	}

	// The reverse edge as REQUIRED would close a two-capability cycle --
	// the whole-graph check must reject it, not merely a single-edge check.
	reverse := capabilitydomain.CapabilityDependency{CapabilityKey: "finance.invoice.issue", DependsOnCapability: "commerce.order.create", DependencyType: capabilitydomain.DependencyTypeRequired}
	if err := repo.CreateCapabilityDependency(ctx, reverse); err == nil {
		t.Fatal("expected a two-capability REQUIRED cycle to be rejected")
	}

	deps, err := repo.ListCapabilityDependencies(ctx, "commerce.order.create")
	if err != nil {
		t.Fatalf("list dependencies failed: %v", err)
	}
	if len(deps) != 1 || deps[0].DependsOnCapability != "finance.invoice.issue" {
		t.Fatalf("expected exactly one dependency, got %+v", deps)
	}
}

func TestInMemoryRepositoryPersistsAndExpiresResolvedContexts(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	resolved := domain.Context{
		ID:            "context-1",
		PrincipalID:   "principal-abc",
		TenantID:      "tenant-123",
		CorrelationID: "correlation-123",
		ResolvedAt:    now,
		Provenance: map[string]domain.ContextSource{
			"tenant_id": {Source: "verified_token", TrustLevel: domain.TrustVerified},
		},
	}
	if err := repo.CreateContext(ctx, resolved); err != nil {
		t.Fatalf("create context failed: %v", err)
	}
	if err := repo.CreateContext(ctx, resolved); err == nil {
		t.Fatal("expected creating a duplicate context id to fail")
	}

	fetched, err := repo.GetContext(ctx, "context-1")
	if err != nil {
		t.Fatalf("get context failed: %v", err)
	}
	if fetched.TenantID != "tenant-123" || fetched.PrincipalID != "principal-abc" {
		t.Fatalf("unexpected fetched context: %+v", fetched)
	}

	if _, err := repo.GetContext(ctx, "missing-context"); !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected ErrContextNotFound for a missing context, got %v", err)
	}

	expiresAt := now.Add(-time.Minute)
	expired := domain.Context{
		ID:            "context-2",
		PrincipalID:   "principal-abc",
		TenantID:      "tenant-123",
		CorrelationID: "correlation-456",
		ResolvedAt:    now.Add(-time.Hour),
		ExpiresAt:     &expiresAt,
		Provenance: map[string]domain.ContextSource{
			"tenant_id": {Source: "verified_token", TrustLevel: domain.TrustVerified},
		},
	}
	// Bypass CreateContext's own Validate() (which itself rejects an
	// expires_at before resolved_at) to seed an already-expired row the way
	// a row aging past its TTL would look, exercising GetContext's own
	// expiry check independently of write-time validation.
	repo.Contexts[expired.ID] = expired
	if _, err := repo.GetContext(ctx, "context-2"); !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected an expired context to report ErrContextNotFound, got %v", err)
	}

	// DeleteContextsByTenant purges every row for the tenant, expired or
	// not: a tenant.suspended event (ADR-BCP-004 §74) means "this tenant's
	// cached contexts are no longer trustworthy," not "except the ones that
	// already expired on their own."
	removed, err := repo.DeleteContextsByTenant(ctx, "tenant-123")
	if err != nil {
		t.Fatalf("delete contexts by tenant failed: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected both contexts for the tenant removed, got %d", removed)
	}
	if _, err := repo.GetContext(ctx, "context-1"); !errors.Is(err, ErrContextNotFound) {
		t.Fatal("expected the deleted context to be gone")
	}
}

func TestInMemoryRepositoryDigitalEstates(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()

	estate := domain.DigitalEstate{ID: "estate-1", TenantID: "tn_zuribeans", Name: "ZuriBeans Storefront", Domain: "shop.zuribeans.com", Status: domain.DigitalEstateActive}
	if err := repo.CreateDigitalEstate(ctx, estate); err != nil {
		t.Fatalf("create digital estate failed: %v", err)
	}
	if err := repo.CreateDigitalEstate(ctx, estate); err == nil {
		t.Fatal("expected duplicate digital estate id to be rejected")
	}

	duplicateDomain := domain.DigitalEstate{ID: "estate-2", TenantID: "tn_other", Name: "Other", Domain: "shop.zuribeans.com", Status: domain.DigitalEstateActive}
	if err := repo.CreateDigitalEstate(ctx, duplicateDomain); err == nil {
		t.Fatal("expected a globally duplicate domain to be rejected")
	}

	fetched, err := repo.GetDigitalEstate(ctx, "estate-1")
	if err != nil {
		t.Fatalf("get digital estate failed: %v", err)
	}
	if fetched.TenantID != "tn_zuribeans" || fetched.Domain != "shop.zuribeans.com" {
		t.Fatalf("unexpected fetched digital estate: %+v", fetched)
	}
	if _, err := repo.GetDigitalEstate(ctx, "missing-estate"); err == nil {
		t.Fatal("expected missing digital estate lookup to fail")
	}

	second := domain.DigitalEstate{ID: "estate-3", TenantID: "tn_zuribeans", Name: "ZuriBeans Wholesale", Domain: "wholesale.zuribeans.com", Status: domain.DigitalEstateActive}
	if err := repo.CreateDigitalEstate(ctx, second); err != nil {
		t.Fatalf("create second digital estate failed: %v", err)
	}
	forOther := domain.DigitalEstate{ID: "estate-4", TenantID: "tn_other", Name: "Other Estate", Domain: "shop.other.com", Status: domain.DigitalEstateActive}
	if err := repo.CreateDigitalEstate(ctx, forOther); err != nil {
		t.Fatalf("create other-tenant digital estate failed: %v", err)
	}

	estates, err := repo.ListDigitalEstatesForTenant(ctx, "tn_zuribeans")
	if err != nil {
		t.Fatalf("list digital estates failed: %v", err)
	}
	if len(estates) != 2 {
		t.Fatalf("expected exactly two digital estates for tn_zuribeans, got %+v", estates)
	}
}
