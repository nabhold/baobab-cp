package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/service"
	"github.com/nabhold/baobab-cp/internal/store"
)

const testTenantID = "tn_01k4zuribeans"

type fakeStore struct {
	calls              int
	registeredTenantID string
	resolveErr         error
	resolved           domain.ResolvedContext
	resolvedCalls      int
	tenant             domain.Tenant
	tenantErr          error
	entitlement        domain.Entitlement
	entitlementErr     error
	entitlementCall    int
	metadata           store.RequestMetadata
}

func (f *fakeStore) Ping(context.Context) error { return nil }
func (f *fakeStore) RegisterTenant(_ context.Context, _ string, metadata store.RequestMetadata, command domain.RegisterTenant) (domain.Operation, error) {
	f.calls++
	f.metadata = metadata
	f.registeredTenantID = command.TenantID
	return domain.Operation{OperationID: "7c8f131b-d8ba-4d89-b60b-a187d3944074", TenantID: command.TenantID, State: "accepted", Revision: 1}, nil
}
func (f *fakeStore) ResolveContext(_ context.Context, metadata store.RequestMetadata, tenantID, productID string) (domain.ResolvedContext, error) {
	f.resolvedCalls++
	f.metadata = metadata
	if f.resolveErr != nil {
		return domain.ResolvedContext{}, f.resolveErr
	}
	return domain.ResolvedContext{TenantID: tenantID, EntityID: tenantID, LifecycleStatus: "active", ProductID: productID, Entitled: true, CacheTTLSeconds: 15, CorrelationID: metadata.CorrelationID}, nil
}
func (f *fakeStore) GetTenant(_ context.Context, tenantID string) (domain.Tenant, error) {
	if f.tenantErr != nil {
		return domain.Tenant{}, f.tenantErr
	}
	if f.tenant.TenantID == "" {
		f.tenant = domain.Tenant{TenantID: tenantID, LegalEntityID: tenantID, DisplayName: "Zuri Beans", IsolationStrategy: "schema_per_tenant", ResidencyRegion: "af-south-1", DesiredState: "active", ObservedState: "ready", Revision: 1}
	}
	return f.tenant, nil
}
func (f *fakeStore) GetEntitlement(_ context.Context, tenantID, productID string) (domain.Entitlement, error) {
	f.entitlementCall++
	if f.entitlementErr != nil {
		return domain.Entitlement{}, f.entitlementErr
	}
	if f.entitlement.TenantID == "" {
		f.entitlement = domain.Entitlement{TenantID: tenantID, ProductID: productID, Status: "active", Tier: "standard"}
	}
	return f.entitlement, nil
}
func (f *fakeStore) UpdateTenantLifecycle(_ context.Context, tenantID string, next domain.LifecycleStatus) error {
	if !next.Valid() {
		return domain.NotFoundError("invalid tenant lifecycle status")
	}
	return nil
}

func TestRegisterTenant(t *testing.T) {
	database := &fakeStore{}
	handler := New(Dependencies{Store: database, AdminVerifier: fakeVerifier{principal: adminPrincipal()}})
	body := `{"legal_entity_id":"THAMANI-GLOBAL","display_name":"Zuri Beans","isolation_strategy":"schema_per_tenant","residency_region":"af-south-1"}`
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Idempotency-Key", strings.Repeat("x", 16))
	req.Header.Set("X-Correlation-ID", "7c8f131b-d8ba-4d89-b60b-a187d3944074")
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if database.calls != 1 {
		t.Fatalf("expected one persistence call, got %d", database.calls)
	}
	if database.metadata.ActorID != "admin-123" || database.metadata.CorrelationID != "7c8f131b-d8ba-4d89-b60b-a187d3944074" {
		t.Fatalf("audit metadata not propagated: %#v", database.metadata)
	}
	if !domain.ValidTenantID(database.registeredTenantID) {
		t.Fatalf("expected a Control Plane-minted tenant_id, got %q", database.registeredTenantID)
	}
}

func TestRegisterTenantRejectsClientSuppliedTenantID(t *testing.T) {
	// tenant_id is Control Plane-minted (tenant-registration.schema.json does
	// not accept it as input); a client that supplies one must be rejected,
	// not silently honoured.
	database := &fakeStore{}
	handler := New(Dependencies{Store: database, AdminVerifier: fakeVerifier{principal: adminPrincipal()}})
	body := `{"legal_entity_id":"THAMANI-GLOBAL","tenant_id":"tn_client_supplied","display_name":"Zuri Beans","isolation_strategy":"schema_per_tenant","residency_region":"af-south-1"}`
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Idempotency-Key", strings.Repeat("x", 16))
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected client-supplied tenant_id to be rejected, got status %d: %s", response.Code, response.Body.String())
	}
	if database.calls != 0 {
		t.Fatalf("expected no persistence call for a rejected request, got %d", database.calls)
	}
}

func TestRegisterTenantRejectsInvalidToken(t *testing.T) {
	handler := New(Dependencies{Store: &fakeStore{}, AdminVerifier: fakeVerifier{err: errors.New("invalid")}})
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer invalid")
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d", response.Code)
	}
}

func TestResolveContext(t *testing.T) {
	store := &fakeStore{}
	handler := New(Dependencies{Store: store, WorkloadVerifier: fakeVerifier{principal: workloadPrincipal()}})
	body := `{"product_id":"baobab_trade"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/context/resolve", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer workload-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if store.resolvedCalls != 1 {
		t.Fatalf("expected one resolve call, got %d", store.resolvedCalls)
	}
}

func TestResolveContextFailsClosed(t *testing.T) {
	store := &fakeStore{resolveErr: store.ErrContextDenied}
	handler := New(Dependencies{Store: store, WorkloadVerifier: fakeVerifier{principal: workloadPrincipal()}})
	req := httptest.NewRequest(http.MethodPost, "/v1/context/resolve", strings.NewReader(`{"product_id":"baobab_trade"}`))
	req.Header.Set("Authorization", "Bearer workload-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
}

func TestGetTenant(t *testing.T) {
	store := &fakeStore{}
	handler := New(Dependencies{Store: store, AdminVerifier: fakeVerifier{principal: adminPrincipal()}})
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/"+testTenantID, nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() == "" {
		t.Fatal("empty tenant response")
	}
}

func TestGetEntitlement(t *testing.T) {
	store := &fakeStore{}
	handler := New(Dependencies{Store: store, AdminVerifier: fakeVerifier{principal: adminPrincipal()}})
	req := httptest.NewRequest(http.MethodGet, "/v1/entitlements?tenantId="+testTenantID+"&productId=baobab-trade", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if store.entitlementCall != 1 {
		t.Fatalf("expected one entitlement lookup, got %d", store.entitlementCall)
	}
}

func TestLifecycleActionEndpoint(t *testing.T) {
	store := &fakeStore{}
	handler := New(Dependencies{Store: store, AdminVerifier: fakeVerifier{principal: adminPrincipal()}})
	for _, tc := range []struct {
		path   string
		method string
	}{
		{path: "/v1/tenants/" + testTenantID + "/suspend", method: http.MethodPost},
		{path: "/v1/tenants/" + testTenantID + "/activate", method: http.MethodPost},
		{path: "/v1/tenants/" + testTenantID + "/decommission", method: http.MethodPost},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("path %s got status %d: %s", tc.path, response.Code, response.Body.String())
		}
	}
}

func TestResolverRouteIsRegistered(t *testing.T) {
	handler := New(Dependencies{
		Store:            &fakeStore{},
		WorkloadVerifier: fakeVerifier{principal: workloadPrincipal()},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer workload-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected registered resolver route to reject empty input with 400, got %d: %s", response.Code, response.Body.String())
	}
}

// TestPlatformContextRouteIsRegisteredDistinctFromLegacyContextResolve is a
// regression test for the naming collision PlatformContextHandler's doc
// comment describes: /v1/context/resolve (the pre-existing, unrelated
// tenant+product entitlement endpoint) must keep working exactly as before,
// while the new ADR-BCP-004 PlatformContext resolver lives at its own path
// and is independently reachable.
func TestPlatformContextRouteIsRegisteredDistinctFromLegacyContextResolve(t *testing.T) {
	store := &fakeStore{}
	handler := New(Dependencies{
		Store:            store,
		WorkloadVerifier: fakeVerifier{principal: workloadPrincipal()},
	})

	legacyReq := httptest.NewRequest(http.MethodPost, "/v1/context/resolve", strings.NewReader(`{"product_id":"baobab_trade"}`))
	legacyReq.Header.Set("Authorization", "Bearer workload-token")
	legacyResponse := httptest.NewRecorder()
	handler.ServeHTTP(legacyResponse, legacyReq)
	if legacyResponse.Code != http.StatusOK {
		t.Fatalf("expected the pre-existing /v1/context/resolve to keep working, got %d: %s", legacyResponse.Code, legacyResponse.Body.String())
	}
	if store.resolvedCalls != 1 {
		t.Fatalf("expected the legacy handler to have been called, got %d calls", store.resolvedCalls)
	}

	// No Identity dependency is configured, so the new route fails closed on
	// identity resolution rather than 404/405 -- either way proves the route
	// is registered and distinct from the legacy one above.
	newReq := httptest.NewRequest(http.MethodPost, "/v1/platform-context/resolve", nil)
	newReq.Header.Set("Authorization", "Bearer workload-token")
	newResponse := httptest.NewRecorder()
	handler.ServeHTTP(newResponse, newReq)
	if newResponse.Code == http.StatusNotFound || newResponse.Code == http.StatusMethodNotAllowed {
		t.Fatalf("expected /v1/platform-context/resolve to be a registered route, got %d: %s", newResponse.Code, newResponse.Body.String())
	}
}

func TestCapabilitiesResolveRouteIsRegistered(t *testing.T) {
	handler := New(Dependencies{
		Store:            &fakeStore{},
		WorkloadVerifier: fakeVerifier{principal: workloadPrincipal()},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/capabilities/resolve", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer workload-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected registered capabilities-resolve route to reject empty input with 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCanonicalEntityLifecycleRoutes(t *testing.T) {
	canonical := service.CanonicalEntityService{Repository: repository.NewCanonicalRepository()}
	handler := New(Dependencies{Store: &fakeStore{}, AdminVerifier: fakeVerifier{principal: adminPrincipal()}, Canonical: canonical})
	body := `{"id":"entity-1","canonical_key":"tenant:product","entity_type":"PRODUCT","display_name":"Product","authority":"baobab","classification":"INTERNAL"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/canonical-entities", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("ETag") != `"1"` {
		t.Fatalf("create got status %d etag %q: %s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}
	for _, action := range []string{"validate", "activate"} {
		request = httptest.NewRequest(http.MethodPost, "/v1/canonical-entities/entity-1/"+action, nil)
		request.Header.Set("Authorization", "Bearer admin-token")
		request.Header.Set("If-Match", `"1"`)
		if action == "activate" {
			request.Header.Set("If-Match", `"2"`)
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s got status %d: %s", action, response.Code, response.Body.String())
		}
	}
}

type fakeVerifier struct {
	principal auth.Principal
	err       error
}

func (f fakeVerifier) Verify(context.Context, string) (auth.Principal, error) {
	if f.err != nil {
		return auth.Principal{}, f.err
	}
	return f.principal, nil
}

func adminPrincipal() auth.Principal {
	return auth.Principal{Subject: "admin-123", ActorType: "admin", TokenID: "token-123", Scopes: map[string]struct{}{"tenant:write": {}, "tenant:read": {}, "canonical:write": {}, "canonical:read": {}}}
}

func workloadPrincipal() auth.Principal {
	return auth.Principal{Subject: "workload-123", ActorType: "workload", TenantID: testTenantID, ClientID: "client-123", TokenID: "token-456", Scopes: map[string]struct{}{"context:resolve": {}}}
}
