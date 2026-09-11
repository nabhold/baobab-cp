package domain

import "testing"

func TestCapabilityScopeRequiresTenant(t *testing.T) {
	if err := (CapabilityScope{}).Validate(); err == nil {
		t.Fatal("scope without tenant_id accepted")
	}
	if err := (CapabilityScope{TenantID: "tn_zuribeans"}).Validate(); err != nil {
		t.Fatalf("valid tenant-only scope rejected: %v", err)
	}
}

func TestCapabilityScopeRejectsContradictoryCountryLists(t *testing.T) {
	// §15: exclusion always wins, but if it cancels every included
	// country the scope can never match anything -- that must be
	// rejected as contradictory configuration, not silently accepted.
	contradictory := CapabilityScope{
		TenantID:         "tn_zuribeans",
		IncludeCountries: []string{"BW"},
		ExcludeCountries: []string{"BW"},
	}
	if err := contradictory.Validate(); err == nil {
		t.Fatal("include/exclude country lists that cancel out entirely were accepted")
	}

	partiallyOverlapping := CapabilityScope{
		TenantID:         "tn_zuribeans",
		IncludeCountries: []string{"ZA", "BW", "ZM"},
		ExcludeCountries: []string{"BW"},
	}
	if err := partiallyOverlapping.Validate(); err != nil {
		t.Fatalf("scope where exclusion narrows but does not eliminate inclusion rejected: %v", err)
	}
}
