package resolver

import (
	"context"
	"testing"
)

func TestTopologyResolverSelectsActiveInstance(t *testing.T) {
	resolver := TopologyResolverImpl{}
	query := TopologyResolutionQuery{
		Context:                  Context{TenantID: "tenant-123", MarketID: "market-789", CountryCode: "ZA"},
		SelectedEngineInstanceID: "instance-1",
		EngineInstances: []EngineInstance{
			{ID: "instance-1", EngineID: "engine-1", Region: "af-south-1", Environment: "production", Status: "ACTIVE"},
			{ID: "instance-2", EngineID: "engine-1", Region: "af-south-1", Environment: "staging", Status: "ACTIVE"},
		},
	}

	resolved, err := resolver.Resolve(context.Background(), query)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.ID != "instance-1" {
		t.Fatalf("expected instance-1, got %q", resolved.ID)
	}
}

func TestTopologyResolverRejectsUnavailableEngine(t *testing.T) {
	resolver := TopologyResolverImpl{}
	_, err := resolver.Resolve(context.Background(), TopologyResolutionQuery{
		Context:                  Context{TenantID: "tenant-123"},
		SelectedEngineInstanceID: "instance-1",
		EngineInstances: []EngineInstance{{
			ID:          "instance-1",
			EngineID:    "engine-1",
			Region:      "af-south-1",
			Environment: "production",
			Status:      "MAINTENANCE",
		}},
	})
	if err == nil {
		t.Fatal("expected unavailable engine rejection")
	}
}

func TestTopologyResolverRejectsDifferentActiveInstance(t *testing.T) {
	resolver := TopologyResolverImpl{}
	_, err := resolver.Resolve(context.Background(), TopologyResolutionQuery{
		Context:                  Context{TenantID: "tn_zuribeans"},
		SelectedEngineInstanceID: "bound-instance",
		EngineInstances:          []EngineInstance{{ID: "other-instance", Status: "ACTIVE", Environment: "production"}},
	})
	if err == nil {
		t.Fatal("topology resolver routed to an instance not selected by the binding")
	}
}

func TestTopologyResolverEnforcesIsolationAndHealth(t *testing.T) {
	resolver := TopologyResolverImpl{}
	base := TopologyResolutionQuery{
		Context:                  Context{TenantID: "tn_zuribeans", Environment: "production", DeploymentRegion: "af-south-1", IsolationProfileID: "iso-zuribeans"},
		SelectedEngineInstanceID: "erp-za-1",
		EngineInstances: []EngineInstance{{
			ID: "erp-za-1", Status: "ACTIVE", HealthStatus: "HEALTHY", Environment: "production", Region: "af-south-1", ResidencyRegion: "af-south-1", IsolationProfileID: "iso-zuribeans",
		}},
	}
	if _, err := resolver.Resolve(context.Background(), base); err != nil {
		t.Fatalf("eligible bound instance rejected: %v", err)
	}
	base.EngineInstances[0].HealthStatus = "UNHEALTHY"
	if _, err := resolver.Resolve(context.Background(), base); err == nil {
		t.Fatal("unhealthy instance received traffic")
	}
	base.EngineInstances[0].HealthStatus = "HEALTHY"
	base.EngineInstances[0].IsolationProfileID = "iso-thamani"
	if _, err := resolver.Resolve(context.Background(), base); err == nil {
		t.Fatal("wrong-isolation instance received traffic")
	}
}
