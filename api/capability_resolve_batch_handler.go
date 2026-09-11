package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/service"
)

// maxBatchResolveEntities bounds how many canonical entities a single
// POST /v1/capabilities/resolve-batch request may resolve, to keep one
// request from doing unbounded resolution work.
const maxBatchResolveEntities = 50

// CapabilityResolveBatchHandler exposes ADR-BCP-003 §70's batch resolution
// ("Batch resolution SHALL NOT hide per-capability failures... Each SHALL
// receive an independent decision") atop an already-resolved Context,
// redeemed by context_id exactly as CapabilityResolveHandler does.
//
// ADR-BCP-003 §70's own example batches multiple distinct capability keys
// for one context ("commerce.order.create", "inventory.availability.read",
// "payment.authorize"). That axis does not exist yet in this codebase:
// resolver.ResolutionPipeline and CapabilityResolverImpl resolve against a
// single hardcoded capability key ("baobab_trade") everywhere, not a
// registry-selected one per request -- generalizing that is a separate,
// larger refactor of already-live resolution code, not something to fold
// into an API-layer batch endpoint. The batching axis this handler
// implements instead is the one degree of freedom the pipeline already
// supports per context: multiple canonical_entity_ids resolved
// independently under the same redeemed Context, matching §70's "SHALL NOT
// hide per-item failures" requirement for whichever items a batch contains.
type CapabilityResolveBatchHandler struct {
	Contexts repository.ContextRepository
	Service  service.ResolutionService
}

type capabilityResolveBatchRequest struct {
	ContextID          string   `json:"context_id"`
	CanonicalEntityIDs []string `json:"canonical_entity_ids"`
}

type capabilityResolveBatchResultItem struct {
	CanonicalEntityID string         `json:"canonical_entity_id"`
	Status            string         `json:"status"`
	Mapping           map[string]any `json:"mapping,omitempty"`
	Capability        map[string]any `json:"capability,omitempty"`
	Policy            map[string]any `json:"policy,omitempty"`
	Topology          map[string]any `json:"topology,omitempty"`
}

func (h CapabilityResolveBatchHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		problem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST only", false)
		return
	}
	var req capabilityResolveBatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), false)
		return
	}
	if req.ContextID == "" || len(req.CanonicalEntityIDs) == 0 {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "context_id and at least one canonical_entity_id are required", false)
		return
	}
	if len(req.CanonicalEntityIDs) > maxBatchResolveEntities {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "canonical_entity_ids may contain at most 50 entries", false)
		return
	}
	for _, id := range req.CanonicalEntityIDs {
		if id == "" {
			problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "canonical_entity_ids must not contain empty values", false)
			return
		}
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.ActorType != "workload" || !principal.HasScope("context:resolve") {
		problem(w, r, http.StatusUnauthorized, "AUTH_TOKEN_REQUIRED", "verified workload identity is required", false)
		return
	}
	if h.Contexts == nil {
		problem(w, r, http.StatusServiceUnavailable, "CONTEXT_STORE_UNAVAILABLE", "context persistence is temporarily unavailable", true)
		return
	}
	trustedContext, err := h.Contexts.GetContext(r.Context(), req.ContextID)
	if errors.Is(err, repository.ErrContextNotFound) {
		problem(w, r, http.StatusNotFound, "CONTEXT_NOT_FOUND", "the referenced context_id does not exist or has expired", false)
		return
	}
	if err != nil {
		problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "context lookup failed", true)
		return
	}
	// Mirrors CapabilityResolveHandler's identical cross-tenant redemption
	// guard (ADR-BCP-004 §71) -- see resolveWorkloadTenant's doc comment
	// for why this isn't a bare equality check.
	if _, ok := resolveWorkloadTenant(principal.TenantID, trustedContext.TenantID); !ok {
		problem(w, r, http.StatusForbidden, "TENANT_CONTEXT_MISMATCH", "the referenced context does not belong to the authenticated tenant", false)
		return
	}

	results := make([]capabilityResolveBatchResultItem, 0, len(req.CanonicalEntityIDs))
	for _, canonicalEntityID := range req.CanonicalEntityIDs {
		result, err := h.Service.Resolve(r.Context(), service.ResolutionRequest{
			TenantID:          trustedContext.TenantID,
			CanonicalEntityID: canonicalEntityID,
			Context:           trustedContext,
		})
		if err != nil {
			// §70: a per-item failure is reported alongside the others, not
			// hidden by aborting the whole batch. ADR-0008 §45 still applies
			// per item: no internal resolver detail is exposed.
			results = append(results, capabilityResolveBatchResultItem{CanonicalEntityID: canonicalEntityID, Status: "DENIED"})
			continue
		}
		results = append(results, capabilityResolveBatchResultItem{
			CanonicalEntityID: canonicalEntityID,
			Status:            "RESOLVED",
			Mapping: map[string]any{
				"id":     result.Mapping.Mapping.ID,
				"status": result.Mapping.Mapping.Status,
			},
			Capability: map[string]any{
				"binding_mode":       result.Capability.BindingMode,
				"engine_instance_id": result.Capability.EngineInstanceID,
			},
			Policy: map[string]any{
				"allowed": result.Policy.Allowed,
				"reason":  result.Policy.Reason,
			},
			Topology: map[string]any{
				"id":          result.Topology.ID,
				"environment": result.Topology.Environment,
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"context_id": trustedContext.ID,
		"tenant_id":  trustedContext.TenantID,
		"results":    results,
	})
}
