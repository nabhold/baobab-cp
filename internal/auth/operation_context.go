package auth

import (
	"context"
	"errors"
	"time"

	"github.com/nabhold/baobab-cp/internal/domain"
)

type operationContextKey struct{}

// NewOperationContext derives the identity portion of Context exclusively
// from a verified Principal. Business dimensions are enriched later from
// authoritative Control Plane registries, never from request headers/body.
func NewOperationContext(parent context.Context, principal Principal, correlationID string, now time.Time) (context.Context, domain.Context, error) {
	if parent == nil {
		return nil, domain.Context{}, errors.New("parent context is required")
	}
	if principal.Subject == "" || principal.TenantID == "" || principal.TokenID == "" {
		return nil, domain.Context{}, errors.New("verified workload principal is required")
	}
	resolved := domain.Context{
		PrincipalID:   principal.Subject,
		TenantID:      principal.TenantID,
		CorrelationID: correlationID,
		ResolvedAt:    now.UTC(),
		Provenance: map[string]domain.ContextSource{
			"principal_id": {Source: "verified_access_token", TrustLevel: domain.TrustVerified, Evidence: principal.TokenID},
			"tenant_id":    {Source: "verified_access_token", TrustLevel: domain.TrustVerified, Evidence: principal.TokenID},
		},
	}
	if err := resolved.Validate(); err != nil {
		return nil, domain.Context{}, err
	}
	return context.WithValue(parent, operationContextKey{}, resolved), resolved, nil
}

// OperationContextFromContext supports immutable propagation to workers and
// internal calls. Callers receive a value copy; provenance is copied as well.
func OperationContextFromContext(ctx context.Context) (domain.Context, bool) {
	resolved, ok := ctx.Value(operationContextKey{}).(domain.Context)
	if !ok {
		return domain.Context{}, false
	}
	resolved.Provenance = cloneProvenance(resolved.Provenance)
	return resolved, true
}

func cloneProvenance(input map[string]domain.ContextSource) map[string]domain.ContextSource {
	output := make(map[string]domain.ContextSource, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
