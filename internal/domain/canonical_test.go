package domain

import "testing"

func TestCanonicalEntityValidate(t *testing.T) {
	valid := CanonicalEntity{
		CanonicalKey:   "tenant:zuribeans_za",
		EntityType:     "TENANT",
		DisplayName:    "Zuri Beans",
		Authority:      "baobab",
		Classification: "INTERNAL",
		Status:         "ACTIVE",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid canonical entity rejected: %v", err)
	}

	invalid := valid
	invalid.CanonicalKey = "bad key"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid canonical key accepted")
	}

	invalid = valid
	invalid.Status = "UNKNOWN"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid canonical status accepted")
	}
}

func TestMappingTypeDefinitionValidate(t *testing.T) {
	valid := MappingTypeDefinition{
		MappingType:      "IDENTITY",
		ResolutionMode:   "SINGLE",
		Description:      "Direct identity mapping",
		Status:           "ACTIVE",
		RequiresApproval: false,
		CrossTenant:      false,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid mapping type rejected: %v", err)
	}

	invalid := valid
	invalid.ResolutionMode = "INVALID"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid resolution mode accepted")
	}
}

func TestMappingValidate(t *testing.T) {
	valid := Mapping{
		MappingType:             "IDENTITY",
		TenantID:                "tenant-abc",
		CanonicalEntityID:       "tenant-123",
		TargetCanonicalEntityID: "entity-456",
		ScopeID:                 "scope-123",
		Direction:               "BIDIRECTIONAL",
		Cardinality:             "ONE_TO_ONE",
		Authority:               "baobab",
		Confidence:              "CONFIRMED",
		Status:                  "ACTIVE",
		EffectiveFrom:           "2025-01-01T00:00:00Z",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid mapping rejected: %v", err)
	}

	invalid := valid
	invalid.CanonicalEntityID = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("missing canonical entity accepted")
	}

	invalid = valid
	invalid.Direction = "INVALID"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid mapping direction accepted")
	}
}
