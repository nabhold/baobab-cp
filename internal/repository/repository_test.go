package repository

import (
	"context"
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
