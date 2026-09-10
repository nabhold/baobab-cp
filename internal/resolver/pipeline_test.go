package resolver

import (
	"context"
	"testing"

	"github.com/nabhold/baobab-cp/internal/domain"
)

func TestResolutionPipelineBuildsFinalDecision(t *testing.T) {
	pipeline := ResolutionPipeline{}
	result, err := pipeline.Resolve(context.Background(), ResolutionRequest{
		TenantID: "tenant-123",
		Context: Context{
			TenantID:      "tenant-123",
			LegalEntityID: "legal-456",
			MarketID:      "market-789",
			CountryCode:   "ZA",
			CurrencyCode:  "ZAR",
			Locale:        "en-ZA",
		},
		Candidates: []domain.Mapping{{
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
		}},
		Bindings: []CapabilityBinding{{
			CapabilityKey:    "baobab_trade",
			EngineID:         "engine-1",
			EngineInstanceID: "instance-1",
			BindingMode:      "PRIMARY",
			Priority:         100,
			Status:           "ACTIVE",
			ContractVersion:  "v1",
		}},
		EngineInstances: []EngineInstance{{
			ID:          "instance-1",
			EngineID:    "engine-1",
			Region:      "af-south-1",
			Environment: "production",
			Status:      "ACTIVE",
		}},
	})
	if err != nil {
		t.Fatalf("pipeline resolve failed: %v", err)
	}
	if result.Context.TenantID != "tenant-123" {
		t.Fatal("tenant context was not preserved")
	}
	if result.Mapping.Mapping.ID != "mapping-tenant" {
		t.Fatal("expected tenant mapping in final resolution")
	}
	if result.Capability.EngineInstanceID != "instance-1" {
		t.Fatal("expected selected engine instance in final resolution")
	}
	if !result.Policy.Allowed {
		t.Fatal("expected policy to allow final decision")
	}
}

func TestResolutionPipelineRejectsMissingTenant(t *testing.T) {
	pipeline := ResolutionPipeline{}
	_, err := pipeline.Resolve(context.Background(), ResolutionRequest{
		TenantID: "",
		Context:  Context{},
	})
	if err == nil {
		t.Fatal("expected missing tenant context rejection")
	}
}
