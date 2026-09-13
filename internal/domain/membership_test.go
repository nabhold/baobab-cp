package domain

import "testing"

func validMembership() WorkforceMembership {
	return WorkforceMembership{
		ID:          NewWorkforceMembershipID(),
		PrincipalID: NewPrincipalID(),
		TenantID:    "tn_01k4m7x9q2v6c8r3d5f1h0j4",
		Roles:       []string{"cp:tenant-admin"},
		Status:      "ACTIVE",
	}
}

func TestWorkforceMembershipValidateRequiresPrincipalID(t *testing.T) {
	m := validMembership()
	m.PrincipalID = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected missing principal_id to be rejected")
	}
}

func TestWorkforceMembershipValidateRejectsInvalidTenantID(t *testing.T) {
	m := validMembership()
	m.TenantID = "zuribeans_za"
	if err := m.Validate(); err == nil {
		t.Fatal("expected non-canonical tenant_id to be rejected")
	}
}

func TestWorkforceMembershipValidateRequiresAtLeastOneRole(t *testing.T) {
	m := validMembership()
	m.Roles = nil
	if err := m.Validate(); err == nil {
		t.Fatal("expected empty roles to be rejected")
	}
}

func TestWorkforceMembershipValidateRejectsEmptyRole(t *testing.T) {
	m := validMembership()
	m.Roles = []string{"cp:tenant-admin", ""}
	if err := m.Validate(); err == nil {
		t.Fatal("expected an empty role string to be rejected")
	}
}

func TestWorkforceMembershipValidateRejectsUnknownStatus(t *testing.T) {
	m := validMembership()
	m.Status = "active"
	if err := m.Validate(); err == nil {
		t.Fatal("expected lowercase status to be rejected (this codebase's lifecycle vocabulary is uppercase)")
	}
	for _, status := range []string{"ACTIVE", "SUSPENDED", "DISABLED"} {
		m.Status = status
		if err := m.Validate(); err != nil {
			t.Fatalf("valid status %q rejected: %v", status, err)
		}
	}
}

func TestWorkforceMembershipValidateAcceptsValidMembership(t *testing.T) {
	if err := validMembership().Validate(); err != nil {
		t.Fatalf("expected a valid membership to pass, got %v", err)
	}
}

func TestWorkforceMembershipHasRole(t *testing.T) {
	m := validMembership()
	m.Roles = []string{"cp:tenant-admin", "cp:billing-viewer"}
	if !m.HasRole("cp:tenant-admin") {
		t.Fatal("expected HasRole to find an assigned role")
	}
	if m.HasRole("cp:platform-admin") {
		t.Fatal("did not expect HasRole to find an unassigned role")
	}
}

func TestValidWorkforceMembershipStatus(t *testing.T) {
	for _, status := range []string{"ACTIVE", "SUSPENDED", "DISABLED"} {
		if !ValidWorkforceMembershipStatus(status) {
			t.Fatalf("expected %q to be a valid status", status)
		}
	}
	for _, status := range []string{"active", "", "ARCHIVED"} {
		if ValidWorkforceMembershipStatus(status) {
			t.Fatalf("did not expect %q to be a valid status", status)
		}
	}
}

func TestNewWorkforceMembershipIDIsUniqueAndUUIDShaped(t *testing.T) {
	a, b := NewWorkforceMembershipID(), NewWorkforceMembershipID()
	if a == b {
		t.Fatal("expected distinct membership IDs")
	}
	if len(a) != 36 {
		t.Fatalf("expected a UUID-shaped (36-char) membership ID, got %q", a)
	}
}
