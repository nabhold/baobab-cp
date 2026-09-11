package api

import "testing"

func TestResolveWorkloadTenant(t *testing.T) {
	cases := []struct {
		name            string
		principalTenant string
		requestedTenant string
		wantTenant      string
		wantOK          bool
	}{
		{
			name:            "no claim, no request -- rejected",
			principalTenant: "",
			requestedTenant: "",
			wantOK:          false,
		},
		{
			name:            "no claim, request supplies tenant -- the real-world workload path",
			principalTenant: "",
			requestedTenant: "tenant-123",
			wantTenant:      "tenant-123",
			wantOK:          true,
		},
		{
			name:            "claim present, no request -- claim wins",
			principalTenant: "tenant-123",
			requestedTenant: "",
			wantTenant:      "tenant-123",
			wantOK:          true,
		},
		{
			name:            "claim present, request matches -- accepted",
			principalTenant: "tenant-123",
			requestedTenant: "tenant-123",
			wantTenant:      "tenant-123",
			wantOK:          true,
		},
		{
			name:            "claim present, request disagrees -- spoofing rejected",
			principalTenant: "tenant-authorised",
			requestedTenant: "tenant-attacker",
			wantOK:          false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenant, ok := resolveWorkloadTenant(tc.principalTenant, tc.requestedTenant)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && tenant != tc.wantTenant {
				t.Fatalf("tenant = %q, want %q", tenant, tc.wantTenant)
			}
		})
	}
}
