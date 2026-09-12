package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/store/postgres"
)

// TestPostgresWorkforceMembershipLifecycle is Gate IAM-5 phase 3's
// (docs/governance/gate-iam-5-workforce-sso-scope.md) regression test for
// ADR-0009 §27's WorkforceMembership relationship and its UNIQUE(principal_id,
// tenant_id) invariant (§31-32: a mover changes an existing membership row
// rather than accumulating a second one for the same tenant), proven against
// a real PostgreSQL 17 instance rather than the in-memory repository alone.
//
// Set TEST_DATABASE_URL to run it; it is skipped otherwise.
func TestPostgresWorkforceMembershipLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()

	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// tenantID/legalEntityID are derived from a fresh UUID rather than a
	// fixed literal: this test's admin pool is closed by its own defer
	// before t.Cleanup's registered cleanup runs (the same ordering already
	// present in TestPostgresIdentityResolvesAndProvisions), so a leaked
	// cleanup silently leaves rows behind -- a fixed tenant/legal-entity id
	// would then collide with itself on the next run, exactly as a fixed
	// (issuer, subject) pair would for that test. Randomizing sidesteps the
	// collision the same way that test's random principal IDs already do.
	suffix := strings.ReplaceAll(domain.NewWorkforceMembershipID(), "-", "")
	tenantID := "tn_" + suffix
	legalEntityID := "GATE-IAM-5-WFM-TEST-" + suffix
	principalID := domain.NewPrincipalID()

	cleanup := func() {
		admin.Exec(ctx, `DELETE FROM identity.workforce_membership WHERE principal_id = $1::uuid`, principalID)
		admin.Exec(ctx, `DELETE FROM identity.principal WHERE principal_id = $1::uuid`, principalID)
		admin.Exec(ctx, `DELETE FROM tenants WHERE tenant_id = $1`, tenantID)
		admin.Exec(ctx, `DELETE FROM legal_entities WHERE legal_entity_id = $1`, legalEntityID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := admin.Exec(ctx, `INSERT INTO legal_entities(legal_entity_id) VALUES ($1)`, legalEntityID); err != nil {
		t.Fatalf("fixture: create legal entity: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO tenants(tenant_id, legal_entity_id, display_name, isolation_strategy, residency_region) VALUES ($1, $2, 'Gate IAM-5 Workforce Membership Test', 'row_level_security', 'af-south-1')`, tenantID, legalEntityID); err != nil {
		t.Fatalf("fixture: create tenant: %v", err)
	}

	repo, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repo.Close()

	if _, err := repo.GetWorkforceMembership(ctx, principalID, tenantID); !errors.Is(err, ErrWorkforceMembershipNotFound) {
		t.Fatalf("expected ErrWorkforceMembershipNotFound before creation, got %v", err)
	}

	principal := domain.Principal{ID: principalID, ActorType: "human", Status: "ACTIVE"}
	if err := repo.CreateIdentity(ctx, principal); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	membership := domain.WorkforceMembership{
		ID: domain.NewWorkforceMembershipID(), PrincipalID: principalID, TenantID: tenantID,
		Roles: []string{"cp:tenant-admin"}, Status: "ACTIVE",
	}
	if err := repo.CreateWorkforceMembership(ctx, membership); err != nil {
		t.Fatalf("create workforce membership: %v", err)
	}

	// ADR-0009 §31-32: a second membership for the same (principal, tenant)
	// pair must be rejected by the database's own UNIQUE constraint, not
	// merely by application logic -- this is the privilege-accumulation
	// failure mode a mover process must never produce.
	duplicate := domain.WorkforceMembership{
		ID: domain.NewWorkforceMembershipID(), PrincipalID: principalID, TenantID: tenantID,
		Roles: []string{"cp:platform-admin"}, Status: "ACTIVE",
	}
	if err := repo.CreateWorkforceMembership(ctx, duplicate); err == nil {
		t.Fatal("expected the UNIQUE(principal_id, tenant_id) constraint to reject a second membership for the same tenant")
	}

	resolved, err := repo.GetWorkforceMembership(ctx, principalID, tenantID)
	if err != nil {
		t.Fatalf("get workforce membership: %v", err)
	}
	if resolved.ID != membership.ID || resolved.Status != "ACTIVE" || !resolved.HasRole("cp:tenant-admin") {
		t.Fatalf("unexpected resolved membership: %+v", resolved)
	}

	memberships, err := repo.ListWorkforceMemberships(ctx, principalID)
	if err != nil {
		t.Fatalf("list workforce memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].TenantID != tenantID {
		t.Fatalf("unexpected memberships: %+v", memberships)
	}

	if err := repo.SetWorkforceMembershipStatus(ctx, membership.ID, "SUSPENDED"); err != nil {
		t.Fatalf("set workforce membership status: %v", err)
	}
	suspended, err := repo.GetWorkforceMembership(ctx, principalID, tenantID)
	if err != nil {
		t.Fatalf("get workforce membership after suspend: %v", err)
	}
	if suspended.Status != "SUSPENDED" || !suspended.HasRole("cp:tenant-admin") {
		t.Fatalf("expected a status-only transition preserving roles, got %+v", suspended)
	}

	if err := repo.SetWorkforceMembershipStatus(ctx, domain.NewWorkforceMembershipID(), "ACTIVE"); !errors.Is(err, ErrWorkforceMembershipNotFound) {
		t.Fatalf("expected ErrWorkforceMembershipNotFound for an unknown membership id, got %v", err)
	}
}
