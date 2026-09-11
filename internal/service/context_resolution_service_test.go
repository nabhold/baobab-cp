package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/store"
)

type fakeTenantStore struct {
	tenant domain.Tenant
	err    error
}

func (f *fakeTenantStore) RegisterTenant(context.Context, string, store.RequestMetadata, domain.RegisterTenant) (domain.Operation, error) {
	return domain.Operation{}, errors.New("not implemented")
}
func (f *fakeTenantStore) ResolveContext(context.Context, store.RequestMetadata, string, string) (domain.ResolvedContext, error) {
	return domain.ResolvedContext{}, errors.New("not implemented")
}
func (f *fakeTenantStore) GetTenant(_ context.Context, tenantID string) (domain.Tenant, error) {
	if f.err != nil {
		return domain.Tenant{}, f.err
	}
	return f.tenant, nil
}
func (f *fakeTenantStore) GetEntitlement(context.Context, string, string) (domain.Entitlement, error) {
	return domain.Entitlement{}, errors.New("not implemented")
}
func (f *fakeTenantStore) UpdateTenantLifecycle(context.Context, string, domain.LifecycleStatus) error {
	return errors.New("not implemented")
}
func (f *fakeTenantStore) Ping(context.Context) error { return nil }

func workloadPrincipalForContext() auth.Principal {
	return auth.Principal{Subject: "baobab-trade", Issuer: "https://iam.nabhold.com/realms/baobab", ActorType: "workload", TenantID: "tenant-123", ClientID: "baobab-trade", TokenID: "token-123"}
}

func activeTenant() domain.Tenant {
	return domain.Tenant{TenantID: "tenant-123", LegalEntityID: "THAMANI-GLOBAL", DisplayName: "Zuri Beans", IsolationStrategy: "schema_per_tenant", ResidencyRegion: "af-south-1", DesiredState: string(domain.LifecycleActive), ObservedState: string(domain.LifecycleActive), Revision: 1}
}

func identityServiceFor(repo *repository.Repository) IdentityService {
	return IdentityService{Repository: repo, Provision: WorkloadOnlyProvisioningPolicy}
}

func TestContextResolutionServiceResolvesTenantAndLegalEntity(t *testing.T) {
	svc := ContextResolutionService{
		Identity: identityServiceFor(repository.NewInMemoryRepository()),
		Tenants:  &fakeTenantStore{tenant: activeTenant()},
	}
	_, resolved, err := svc.Resolve(context.Background(), workloadPrincipalForContext(), "correlation-123", time.Now())
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.TenantID != "tenant-123" {
		t.Fatalf("expected tenant_id tenant-123, got %q", resolved.TenantID)
	}
	if resolved.LegalEntityID != "THAMANI-GLOBAL" {
		t.Fatalf("expected legal_entity_id to be populated from the tenant record, got %q", resolved.LegalEntityID)
	}
}

func TestContextResolutionServiceRejectsUnknownTenant(t *testing.T) {
	svc := ContextResolutionService{
		Identity: identityServiceFor(repository.NewInMemoryRepository()),
		Tenants:  &fakeTenantStore{err: domain.NotFoundError("tenant not found")},
	}
	if _, _, err := svc.Resolve(context.Background(), workloadPrincipalForContext(), "correlation-123", time.Now()); err == nil {
		t.Fatal("expected an unknown tenant to be rejected")
	}
}

func TestContextResolutionServiceRejectsInactiveTenant(t *testing.T) {
	tenant := activeTenant()
	tenant.ObservedState = string(domain.LifecycleSuspended)
	svc := ContextResolutionService{
		Identity: identityServiceFor(repository.NewInMemoryRepository()),
		Tenants:  &fakeTenantStore{tenant: tenant},
	}
	_, _, err := svc.Resolve(context.Background(), workloadPrincipalForContext(), "correlation-123", time.Now())
	if !errors.Is(err, ErrTenantNotActive) {
		t.Fatalf("expected ErrTenantNotActive for a suspended tenant, got %v", err)
	}
}

func TestContextResolutionServiceWrapsIdentityFailure(t *testing.T) {
	// No Provision policy: IdentityService fails closed on an unknown
	// principal.
	svc := ContextResolutionService{
		Identity: IdentityService{Repository: repository.NewInMemoryRepository()},
		Tenants:  &fakeTenantStore{tenant: activeTenant()},
	}
	_, _, err := svc.Resolve(context.Background(), workloadPrincipalForContext(), "correlation-123", time.Now())
	if !errors.Is(err, ErrIdentityResolutionFailed) {
		t.Fatalf("expected ErrIdentityResolutionFailed, got %v", err)
	}
}

func TestContextResolutionServiceRequiresTenantStore(t *testing.T) {
	svc := ContextResolutionService{Identity: identityServiceFor(repository.NewInMemoryRepository())}
	if _, _, err := svc.Resolve(context.Background(), workloadPrincipalForContext(), "correlation-123", time.Now()); err == nil {
		t.Fatal("expected a nil Tenants store to be rejected")
	}
}
