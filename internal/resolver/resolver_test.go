package resolver

import "testing"

func TestScopeMatcherPrefersMoreSpecificMatch(t *testing.T) {
	matcher := DefaultScopeMatcher{}
	ctx := Context{
		TenantID:      "tenant-123",
		LegalEntityID: "legal-456",
		MarketID:      "market-789",
		CountryCode:   "ZA",
		CurrencyCode:  "ZAR",
		Locale:        "en-ZA",
	}

	scope := ScopeValues{
		TenantID:      "tenant-123",
		LegalEntityID: "legal-456",
		MarketID:      "market-789",
		CountryCode:   "ZA",
		CurrencyCode:  "ZAR",
		Locale:        "en-ZA",
	}

	match := matcher.Match(ctx, scope)
	if !match.Compatible {
		t.Fatal("expected compatible scope match")
	}
	if match.Specificity == 0 {
		t.Fatal("expected positive specificity")
	}
	if len(match.Matched) == 0 {
		t.Fatal("expected at least one matched dimension")
	}
}

func TestContextResolverMergesEvidenceAndTrust(t *testing.T) {
	resolver := ContextResolverImpl{}
	evidence := ResolutionEvidence{
		PrincipalID:   "baobab-trade",
		TenantID:      "tenant-123",
		LegalEntityID: "legal-456",
		MarketID:      "market-789",
		CountryCode:   "ZA",
		CurrencyCode:  "ZAR",
		Locale:        "en-ZA",
		CorrelationID: "correlation-123",
		Provenance: map[string]ContextSource{
			"tenant":    {Source: "authn", TrustLevel: TrustAuthorised, Evidence: "jwt-subject"},
			"principal": {Source: "authn", TrustLevel: TrustAuthorised, Evidence: "jwt-subject"},
		},
	}

	ctx, err := resolver.Resolve(t.Context(), evidence)
	if err != nil {
		t.Fatalf("context resolution failed: %v", err)
	}
	if ctx.TenantID != "tenant-123" {
		t.Fatalf("tenant mismatch: got %q", ctx.TenantID)
	}
	if ctx.Provenance["tenant"].TrustLevel != TrustAuthorised {
		t.Fatal("expected authorised provenance")
	}
}
