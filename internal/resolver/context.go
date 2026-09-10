package resolver

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nabhold/baobab-cp/internal/domain"
)

// Context represents a trusted runtime context produced by the resolver layer.
type Context = domain.Context

// TrustLevel identifies the trust level of the source evidence used to derive a context value.
type TrustLevel = domain.TrustLevel

const (
	TrustUntrusted  = domain.TrustUntrusted
	TrustVerified   = domain.TrustVerified
	TrustAuthorised = domain.TrustAuthorised
	TrustSystem     = domain.TrustSystem
)

type ContextSource = domain.ContextSource

// ResolutionEvidence is the input used by the resolver to derive runtime context.
type ResolutionEvidence struct {
	PrincipalID   string
	TenantID      string
	LegalEntityID string
	MarketID      string
	CountryCode   string
	CurrencyCode  string
	Locale        string
	CorrelationID string
	Provenance    map[string]ContextSource
}

// ContextResolverImpl resolves runtime context using supplied evidence and trust metadata.
type ContextResolverImpl struct{}

func (ContextResolverImpl) Resolve(ctx context.Context, evidence ResolutionEvidence) (Context, error) {
	if ctx == nil {
		return Context{}, errors.New("context is required")
	}
	if evidence.TenantID == "" {
		return Context{}, errors.New("tenant_id is required")
	}
	if evidence.PrincipalID == "" || evidence.CorrelationID == "" {
		return Context{}, errors.New("principal_id and correlation_id are required")
	}
	if evidence.Provenance == nil {
		evidence.Provenance = map[string]ContextSource{}
	}
	for key, value := range evidence.Provenance {
		if value.TrustLevel == "" {
			value.TrustLevel = TrustUntrusted
			evidence.Provenance[key] = value
		}
	}
	resolved := Context{
		PrincipalID:   evidence.PrincipalID,
		TenantID:      evidence.TenantID,
		LegalEntityID: evidence.LegalEntityID,
		MarketID:      evidence.MarketID,
		CountryCode:   evidence.CountryCode,
		CurrencyCode:  evidence.CurrencyCode,
		Locale:        evidence.Locale,
		CorrelationID: evidence.CorrelationID,
		ResolvedAt:    time.Now().UTC(),
		Provenance:    evidence.Provenance,
	}
	if err := resolved.Validate(); err != nil {
		return Context{}, err
	}
	return resolved, nil
}

// ScopeMatch is the result of comparing a context against a scope.
type ScopeMatch struct {
	Compatible  bool
	Specificity int
	Matched     []string
	Inherited   []string
	RejectedBy  []string
}

// DefaultScopeMatcher is the default deterministic scope matcher.
type DefaultScopeMatcher struct{}

// Match compares a Context against a domain.MappingScope directly -- this
// used to take a package-local ScopeValues struct that duplicated
// domain.MappingScope's own dimension fields field-for-field, requiring a
// manual conversion at every call site (audit finding: "Runtime resolver
// types duplicate canonical concepts", docs/reconciliation/
// platform-resolution-spine-audit.md Gate 1). domain.MappingScope is the
// one canonical model for this concept; ID and EntityType (the two fields
// ScopeValues didn't carry) are simply unused by the matcher below.
func (DefaultScopeMatcher) Match(ctx Context, scope domain.MappingScope) ScopeMatch {
	matched := []string{}
	inherited := []string{}
	rejectedBy := []string{}
	specificity := 0

	matchChecks := []struct {
		name  string
		value string
		want  string
	}{
		{name: "tenant", value: ctx.TenantID, want: scope.TenantID},
		{name: "legal_entity", value: ctx.LegalEntityID, want: scope.LegalEntityID},
		{name: "market", value: ctx.MarketID, want: scope.MarketID},
		{name: "country", value: ctx.CountryCode, want: scope.CountryCode},
		{name: "digital_estate", value: ctx.DigitalEstateID, want: scope.DigitalEstateID},
		{name: "digital_property", value: ctx.DigitalPropertyID, want: scope.DigitalPropertyID},
		{name: "channel", value: ctx.ChannelID, want: scope.ChannelID},
		{name: "currency", value: ctx.CurrencyCode, want: scope.CurrencyCode},
		{name: "locale", value: ctx.Locale, want: scope.Locale},
		{name: "environment", value: ctx.Environment, want: scope.Environment},
	}

	for _, check := range matchChecks {
		if check.want == "" {
			continue
		}
		if check.value == check.want {
			matched = append(matched, check.name)
			specificity++
			continue
		}
		if check.value != "" && check.value != check.want {
			rejectedBy = append(rejectedBy, check.name)
			continue
		}
		inherited = append(inherited, check.name)
	}

	compatible := len(rejectedBy) == 0
	if compatible && len(matched) == 0 && len(inherited) == 0 {
		compatibility := ScopeMatch{Compatible: true, Specificity: 0, Matched: matched, Inherited: inherited, RejectedBy: rejectedBy}
		return compatibility
	}

	sort.Strings(matched)
	sort.Strings(inherited)
	sort.Strings(rejectedBy)

	return ScopeMatch{Compatible: compatible, Specificity: specificity, Matched: matched, Inherited: inherited, RejectedBy: rejectedBy}
}
