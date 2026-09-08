package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/resolver"
	"github.com/nabhold/baobab-cp/internal/service"
)

func TestResolverHandlerResolve(t *testing.T) {
	serviceInst := service.ResolutionService{Pipeline: resolver.ResolutionPipeline{}}
	handler := ResolverHandler{Service: serviceInst}

	body := map[string]any{
		"tenant_id": "tenant-123",
		"context": map[string]any{
			"tenant_id":      "tenant-123",
			"legal_entity_id": "legal-456",
			"market_id":      "market-789",
			"country_code":   "ZA",
			"currency_code":  "ZAR",
			"locale":         "en-ZA",
		},
		"mappings": []map[string]any{{
			"id":                      "mapping-tenant",
			"mapping_type":            "IDENTITY",
			"resolution_mode":         "SINGLE",
			"canonical_entity_id":     "tenant-123",
			"target_canonical_entity_id": "entity-tenant",
			"scope_id":                "tenant-123",
			"direction":               "BIDIRECTIONAL",
			"cardinality":             "ONE_TO_ONE",
			"authority":               "baobab",
			"confidence":              "CONFIRMED",
			"status":                  "ACTIVE",
			"resolution_priority":     50,
			"effective_from":          "2025-01-01T00:00:00Z",
		}},
		"bindings": []map[string]any{{
			"capability_key":     "baobab_trade",
			"engine_id":          "engine-1",
			"engine_instance_id": "instance-1",
			"binding_mode":       "PRIMARY",
			"priority":           100,
			"status":             "ACTIVE",
			"contract_version":   "v1",
		}},
		"engine_instances": []map[string]any{{
			"id":          "instance-1",
			"engine_id":   "engine-1",
			"region":      "af-south-1",
			"environment": "production",
			"status":      "ACTIVE",
		}},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewReader(payload))
	principal := auth.Principal{Subject: "baobab-trade", ActorType: "workload", TenantID: "tenant-123", ClientID: "baobab-trade", TokenID: "token-123"}
	requestContext := context.WithValue(context.Background(), correlationKey{}, "00000000-0000-4000-8000-000000000001")
	req = req.WithContext(auth.WithPrincipal(requestContext, principal))
	w := httptest.NewRecorder()

	handler.Resolve(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("tenant-123")) {
		t.Fatalf("expected tenant in response, got %s", w.Body.String())
	}
}

func TestResolverHandlerRejectsBadInput(t *testing.T) {
	handler := ResolverHandler{Service: service.ResolutionService{Pipeline: resolver.ResolutionPipeline{}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewReader([]byte(`{"tenant_id":""}`)))
	w := httptest.NewRecorder()

	handler.Resolve(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestResolverHandlerRejectsTenantSpoofing(t *testing.T) {
	handler := ResolverHandler{Service: service.ResolutionService{Pipeline: resolver.ResolutionPipeline{}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewReader([]byte(`{"tenant_id":"tenant-attacker"}`)))
	principal := auth.Principal{Subject: "baobab-trade", ActorType: "workload", TenantID: "tenant-authorised", ClientID: "baobab-trade", TokenID: "token-123"}
	req = req.WithContext(auth.WithPrincipal(context.Background(), principal))
	w := httptest.NewRecorder()

	handler.Resolve(w, req)
	if w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("TENANT_CONTEXT_MISMATCH")) {
		t.Fatalf("expected fail-closed tenant mismatch, got %d body=%s", w.Code, w.Body.String())
	}
}

var _ = domain.RegisterTenant{}
