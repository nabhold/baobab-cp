package domain

import "testing"

func TestDigitalEstateValidate(t *testing.T) {
	valid := DigitalEstate{TenantID: "tn_zuribeans", Name: "ZuriBeans Storefront", Domain: "shop.zuribeans.com", Status: DigitalEstateActive}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid digital estate rejected: %v", err)
	}

	missingTenant := valid
	missingTenant.TenantID = ""
	if err := missingTenant.Validate(); err == nil {
		t.Fatal("expected missing tenant_id to be rejected")
	}

	missingName := valid
	missingName.Name = ""
	if err := missingName.Validate(); err == nil {
		t.Fatal("expected missing name to be rejected")
	}

	missingDomain := valid
	missingDomain.Domain = ""
	if err := missingDomain.Validate(); err == nil {
		t.Fatal("expected missing domain to be rejected")
	}
}
