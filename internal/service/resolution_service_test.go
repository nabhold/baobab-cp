package service

import (
	"context"
	"testing"

	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/resolver"
)

func TestResolutionServiceResolve(t *testing.T) {
	service := ResolutionService{Pipeline: resolver.ResolutionPipeline{}}
	result, err := service.Resolve(context.Background(), ResolutionRequest{
		TenantID:          "tenant-123",
		CanonicalEntityID: "entity-abc",
		Context: resolver.Context{
			TenantID:      "tenant-123",
			LegalEntityID: "legal-456",
			MarketID:      "market-789",
			CountryCode:   "ZA",
			CurrencyCode:  "ZAR",
			Locale:        "en-ZA",
		},
		Mappings: []domain.Mapping{{
			ID:                      "mapping-tenant",
			MappingType:             "IDENTITY",
			TenantID:                "tenant-123",
			CanonicalEntityID:       "entity-abc",
			TargetCanonicalEntityID: "entity-tenant",
			ScopeID:                 "tenant-123",
			Direction:               "BIDIRECTIONAL",
			Cardinality:             "ONE_TO_ONE",
			Authority:               "baobab",
			Confidence:              "CONFIRMED",
			Status:                  "ACTIVE",
			ResolutionPriority:      50,
			EffectiveFrom:           "2025-01-01T00:00:00Z",
		}},
		Bindings: []resolver.CapabilityBinding{{
			CapabilityKey:    "baobab_trade",
			EngineID:         "engine-1",
			EngineInstanceID: "instance-1",
			BindingMode:      "PRIMARY",
			Priority:         100,
			Status:           "ACTIVE",
			ContractVersion:  "v1",
		}},
		EngineInstances: []resolver.EngineInstance{{
			ID:          "instance-1",
			EngineID:    "engine-1",
			Region:      "af-south-1",
			Environment: "production",
			Status:      "ACTIVE",
		}},
	})
	if err != nil {
		t.Fatalf("service resolve failed: %v", err)
	}
	if result.Context.TenantID != "tenant-123" {
		t.Fatal("tenant context not preserved")
	}
	if result.Capability.EngineInstanceID != "instance-1" {
		t.Fatal("engine instance was not selected")
	}
}

func TestResolutionServiceRequiresTenant(t *testing.T) {
	service := ResolutionService{Pipeline: resolver.ResolutionPipeline{}}
	_, err := service.Resolve(context.Background(), ResolutionRequest{
		TenantID: "",
		Context:  resolver.Context{},
	})
	if err == nil {
		t.Fatal("expected tenant requirement error")
	}
}

func TestResolutionServiceUsesRepositoryState(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	repo.Mappings["entity-abc"] = []domain.Mapping{{
		ID: "authoritative", MappingType: "IDENTITY", TenantID: "tenant-123", CanonicalEntityID: "entity-abc",
		TargetCanonicalEntityID: "entity-1", ScopeID: "tenant-123", Direction: "BIDIRECTIONAL", Cardinality: "ONE_TO_ONE",
		Authority: "baobab", Confidence: "CONFIRMED", Status: "ACTIVE", EffectiveFrom: "2025-01-01T00:00:00Z",
	}}
	repo.Bindings["baobab_trade"] = []resolver.CapabilityBinding{{CapabilityKey: "baobab_trade", EngineID: "engine-1", EngineInstanceID: "instance-1", BindingMode: "PRIMARY", Status: "ACTIVE", ContractVersion: "v1"}}
	repo.EngineInstances["engine-1"] = []resolver.EngineInstance{{ID: "instance-1", EngineID: "engine-1", Environment: "production", Status: "ACTIVE"}}
	service := ResolutionService{Pipeline: resolver.ResolutionPipeline{}, Repository: repo}
	result, err := service.Resolve(context.Background(), ResolutionRequest{
		TenantID:          "tenant-123",
		CanonicalEntityID: "entity-abc",
		Context: resolver.Context{
			TenantID:    "tenant-123",
			MarketID:    "market-1",
			CountryCode: "ZA",
		},
		Mappings: []domain.Mapping{{ID: "untrusted", CanonicalEntityID: "entity-abc", Status: "ACTIVE"}},
	})
	if err != nil {
		t.Fatalf("repository-backed resolve failed: %v", err)
	}
	if result.Mapping.Mapping.ID != "authoritative" {
		t.Fatalf("expected repository mapping, got %s", result.Mapping.Mapping.ID)
	}
}
