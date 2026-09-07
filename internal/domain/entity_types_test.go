package domain

import "testing"

// TestSupplierOrganisationEntityTypeIsDataDriven proves that registering a
// new canonical entity kind (SUPPLIER_ORGANISATION, see ADR-0006) needs no
// change to CanonicalEntity.Validate or any other domain code: EntityType
// is a plain string, so a caller can use the new constant today.
func TestSupplierOrganisationEntityTypeIsDataDriven(t *testing.T) {
	entity := CanonicalEntity{
		CanonicalKey:   "supplier:thamani_global:sup_01k4p8q2r3s4",
		EntityType:     EntityTypeSupplierOrganisation,
		DisplayName:    "Highland Fresh Produce Ltd",
		Authority:      "baobab",
		Classification: "TENANT_CONFIDENTIAL",
		Status:         "DRAFT",
	}
	if err := entity.Validate(); err != nil {
		t.Fatalf("supplier organisation canonical entity rejected: %v", err)
	}
	if entity.EntityType != "SUPPLIER_ORGANISATION" {
		t.Fatalf("unexpected entity type constant value: %q", entity.EntityType)
	}
}
