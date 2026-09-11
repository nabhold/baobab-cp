package domain

import (
	"testing"
	"time"
)

func TestIsolationProfileValidate(t *testing.T) {
	valid := IsolationProfile{Name: "Default Schema Isolation", Strategy: "schema_per_tenant"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid isolation profile rejected: %v", err)
	}

	missingName := valid
	missingName.Name = ""
	if err := missingName.Validate(); err == nil {
		t.Fatal("expected missing name to be rejected")
	}

	badStrategy := valid
	badStrategy.Strategy = "shared_database"
	if err := badStrategy.Validate(); err == nil {
		t.Fatal("expected an invalid strategy to be rejected")
	}

	rls := valid
	rls.Strategy = "row_level_security"
	if err := rls.Validate(); err != nil {
		t.Fatalf("valid row_level_security strategy rejected: %v", err)
	}
}

func TestTenantIsolationProfileAssignmentValidate(t *testing.T) {
	now := time.Now().UTC()
	valid := TenantIsolationProfileAssignment{TenantID: "tn_zuribeans", IsolationProfileID: "profile-1", EffectiveFrom: now}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid assignment rejected: %v", err)
	}

	missingTenant := valid
	missingTenant.TenantID = ""
	if err := missingTenant.Validate(); err == nil {
		t.Fatal("expected missing tenant_id to be rejected")
	}

	missingProfile := valid
	missingProfile.IsolationProfileID = ""
	if err := missingProfile.Validate(); err == nil {
		t.Fatal("expected missing isolation_profile_id to be rejected")
	}

	missingFrom := valid
	missingFrom.EffectiveFrom = time.Time{}
	if err := missingFrom.Validate(); err == nil {
		t.Fatal("expected missing effective_from to be rejected")
	}

	before := valid
	badTo := now.Add(-time.Hour)
	before.EffectiveTo = &badTo
	if err := before.Validate(); err == nil {
		t.Fatal("expected effective_to at or before effective_from to be rejected")
	}
}
