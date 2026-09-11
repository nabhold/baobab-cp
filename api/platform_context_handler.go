package api

import (
	"net/http"
	"time"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/service"
)

// PlatformContextHandler exposes ADR-BCP-004's context resolution as its own
// step (§75 recommends "POST /v1/context/resolve" for this) -- deliberately
// mounted at a different path here because /v1/context/resolve already
// exists (api/context.go's resolveContext) and resolves an unrelated, older
// concept: tenant+product entitlement (domain.ResolveContext/
// ResolvedContext, predating the capability-centric rebuild), not the
// PlatformContext this handler produces. The two are not renamed or merged
// by this change.
//
// A resolved Context is persisted via repository.ContextWriter so its
// context_id can be redeemed later by CapabilityResolveHandler, per
// nabhold/shared's resolutionRequest contract requiring a pre-resolved
// context_id rather than an inline context.
type PlatformContextHandler struct {
	Identity service.IdentityService
	Contexts repository.ContextWriter
	// TTL bounds the persisted context's lifetime (ADR-BCP-004 §72). Zero
	// (the default) leaves it unbounded, mirroring resolver.
	// ResolutionEvidence.TTL's identical zero-value semantics.
	TTL time.Duration
}

type platformContextResolveResponse struct {
	ContextID  string     `json:"context_id"`
	TenantID   string     `json:"tenant_id"`
	ResolvedAt time.Time  `json:"resolved_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func (h PlatformContextHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		problem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST only", false)
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.ActorType != "workload" || !principal.HasScope("context:resolve") {
		problem(w, r, http.StatusUnauthorized, "AUTH_TOKEN_REQUIRED", "verified workload identity is required", false)
		return
	}
	// ADR-0004 §11/§39: resolve the verified (issuer, subject) to a durable
	// canonical Principal before any business-domain operation proceeds,
	// mirroring ResolverHandler.Resolve's identical identity-resolution step.
	resolvedPrincipal, err := h.Identity.Resolve(r.Context(), principal.Issuer, principal.Subject, principal.ActorType)
	if err != nil {
		problem(w, r, http.StatusForbidden, "IDENTITY_RESOLUTION_FAILED", "the authenticated identity could not be resolved", false)
		return
	}
	_, trustedContext, err := auth.NewOperationContext(r.Context(), principal, resolvedPrincipal.ID, correlationID(r), time.Now())
	if err != nil {
		problem(w, r, http.StatusForbidden, "CONTEXT_DENIED", "trusted Context could not be constructed", false)
		return
	}
	if h.TTL > 0 {
		expiresAt := trustedContext.ResolvedAt.Add(h.TTL)
		trustedContext.ExpiresAt = &expiresAt
	}
	if h.Contexts == nil {
		problem(w, r, http.StatusServiceUnavailable, "CONTEXT_STORE_UNAVAILABLE", "context persistence is temporarily unavailable", true)
		return
	}
	if err := h.Contexts.CreateContext(r.Context(), trustedContext); err != nil {
		problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "resolved context could not be persisted", true)
		return
	}
	writeJSON(w, http.StatusOK, platformContextResolveResponse{
		ContextID:  trustedContext.ID,
		TenantID:   trustedContext.TenantID,
		ResolvedAt: trustedContext.ResolvedAt,
		ExpiresAt:  trustedContext.ExpiresAt,
	})
}
