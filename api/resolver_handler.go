package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/service"
)

// ResolverHandler exposes the composed resolution service over HTTP.
type ResolverHandler struct {
	Service service.ResolutionService
}

type resolverRequest struct {
	TenantID          string `json:"tenant_id"`
	CanonicalEntityID string `json:"canonical_entity_id"`
}

func (h ResolverHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		problem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST only", false)
		return
	}

	var req resolverRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), false)
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.ActorType != "workload" || !principal.HasScope("context:resolve") {
		problem(w, r, http.StatusUnauthorized, "AUTH_TOKEN_REQUIRED", "verified workload identity is required", false)
		return
	}
	requestedTenant := req.TenantID
	if requestedTenant != "" && requestedTenant != principal.TenantID {
		problem(w, r, http.StatusForbidden, "TENANT_CONTEXT_MISMATCH", "requested tenant does not match verified workload identity", false)
		return
	}
	// contracts/control-plane/v1/canonical-mapping.schema.json's
	// resolutionRequest already requires canonical_entity_id -- this
	// repository just never accepted it (docs/governance/gate-iam-0-
	// discovery.md R-3), so nothing downstream could resolve an actual
	// entity, only the tenant itself.
	if req.CanonicalEntityID == "" {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "canonical_entity_id is required", false)
		return
	}
	operationCtx, trustedContext, err := auth.NewOperationContext(r.Context(), principal, correlationID(r), time.Now())
	if err != nil {
		problem(w, r, http.StatusForbidden, "CONTEXT_DENIED", "trusted Context could not be constructed", false)
		return
	}
	r = r.WithContext(operationCtx)

	result, err := h.Service.Resolve(operationCtx, service.ResolutionRequest{
		TenantID:          trustedContext.TenantID,
		CanonicalEntityID: req.CanonicalEntityID,
		Context:           trustedContext,
	})
	if err != nil {
		// ADR-0008 §45 ("Denial Model"): reason codes SHALL not expose
		// sensitive information indiscriminately to external clients. err
		// here is a *resolver.ResolutionError wrapping the pipeline's raw
		// internal cause (mapping/capability/policy/topology internals,
		// e.g. "mapping not found", "engine instance missing") — that detail
		// is an operator diagnostic, not a client-facing message. Mirrors
		// how resolveContext (api/context.go) already treats
		// store.ErrContextDenied with a fixed, opaque detail string.
		problem(w, r, http.StatusBadRequest, "RESOLUTION_FAILED", "the request could not be resolved to an authorized routing decision", false)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tenant_id": result.Context.TenantID,
		"mapping": map[string]any{
			"id":     result.Mapping.Mapping.ID,
			"status": result.Mapping.Mapping.Status,
		},
		"capability": map[string]any{
			"binding_mode":       result.Capability.BindingMode,
			"engine_instance_id": result.Capability.EngineInstanceID,
		},
		"policy": map[string]any{
			"allowed": result.Policy.Allowed,
			"reason":  result.Policy.Reason,
		},
		"topology": map[string]any{
			"id":          result.Topology.ID,
			"environment": result.Topology.Environment,
		},
	})
}
