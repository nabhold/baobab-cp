package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nabhold/baobab-cp/internal/auth"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/repository"
	"github.com/nabhold/baobab-cp/internal/service"
	"github.com/nabhold/baobab-cp/internal/store"
)

type correlationKey struct{}

type Dependencies struct {
	Store            store.TenantStore
	AdminVerifier    auth.TokenVerifier
	WorkloadVerifier auth.TokenVerifier
	Resolution       service.ResolutionService
	Canonical        service.CanonicalEntityService
	Identity         service.IdentityService
	// Contexts backs PlatformContextHandler/CapabilityResolveHandler (the
	// ADR-BCP-004/003 Runtime APIs). Nil is a valid zero value: both
	// handlers return 503 CONTEXT_STORE_UNAVAILABLE rather than panicking
	// when it is unset, so leaving it out of Dependencies (as every existing
	// caller of New does today) changes nothing about any other route.
	Contexts repository.ContextStore
	// PlatformContextTTL bounds how long PlatformContextHandler's persisted
	// contexts remain redeemable (ADR-BCP-004 §72). Zero leaves them
	// unbounded -- callers that don't set it (every test in this package
	// today) keep that prior behavior; cmd/controlplane/main.go sets it from
	// config.Config.PlatformContextTTL, which itself defaults to a bounded
	// value rather than leaving it unset.
	PlatformContextTTL time.Duration
}
type API struct {
	store            store.TenantStore
	adminVerifier    auth.TokenVerifier
	workloadVerifier auth.TokenVerifier
	resolution       service.ResolutionService
}

func New(dependencies Dependencies) http.Handler {
	a := &API{store: dependencies.Store, adminVerifier: dependencies.AdminVerifier, workloadVerifier: dependencies.WorkloadVerifier, resolution: dependencies.Resolution}
	r := chi.NewRouter()
	r.Use(a.securityHeaders, a.correlation, a.requestLog)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", a.ready)
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:write")).Post("/v1/tenants", a.register)
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:read")).Get("/v1/tenants/{tenantID}", a.getTenant)
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:write")).Post("/v1/tenants/{tenantID}/suspend", a.tenantLifecycleAction("suspend"))
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:write")).Post("/v1/tenants/{tenantID}/activate", a.tenantLifecycleAction("activate"))
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:write")).Post("/v1/tenants/{tenantID}/decommission", a.tenantLifecycleAction("decommission"))
	r.With(a.authorize(a.adminVerifier, "admin", "tenant:read")).Get("/v1/entitlements", a.getEntitlement)
	r.With(a.authorize(a.workloadVerifier, "workload", "context:resolve")).Post("/v1/context/resolve", a.resolveContext)
	r.With(a.authorize(a.workloadVerifier, "workload", "context:resolve")).Post("/v1/resolve", ResolverHandler{Service: a.resolution, Identity: dependencies.Identity}.Resolve)
	// ADR-BCP-004/003 Runtime APIs (issue #74 sub-work item 6), deliberately
	// on their own paths rather than /v1/context/resolve and /v1/resolve
	// (which are the pre-existing, differently-shaped endpoints above): see
	// PlatformContextHandler's doc comment for why.
	r.With(a.authorize(a.workloadVerifier, "workload", "context:resolve")).Post("/v1/platform-context/resolve", PlatformContextHandler{Identity: dependencies.Identity, Contexts: dependencies.Contexts, TTL: dependencies.PlatformContextTTL}.Resolve)
	r.With(a.authorize(a.workloadVerifier, "workload", "context:resolve")).Post("/v1/capabilities/resolve", CapabilityResolveHandler{Contexts: dependencies.Contexts, Service: a.resolution}.Resolve)
	canonical := canonicalHandler{service: dependencies.Canonical}
	r.With(a.authorize(a.adminVerifier, "admin", "canonical:write")).Post("/v1/canonical-entities", canonical.create)
	r.With(a.authorize(a.adminVerifier, "admin", "canonical:read")).Get("/v1/canonical-entities/{entityID}", canonical.get)
	for _, action := range []string{"validate", "activate", "suspend", "retire"} {
		r.With(a.authorize(a.adminVerifier, "admin", "canonical:write")).Post("/v1/canonical-entities/{entityID}/"+action, canonical.lifecycle(action))
	}
	return r
}

func (a *API) authorize(verifier auth.TokenVerifier, actorType, requiredScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				problem(w, r, http.StatusUnauthorized, "AUTH_TOKEN_REQUIRED", "a bearer token is required", false)
				return
			}
			if verifier == nil {
				problem(w, r, http.StatusServiceUnavailable, "AUTH_VERIFIER_UNAVAILABLE", "authentication is temporarily unavailable", true)
				return
			}
			principal, err := verifier.Verify(r.Context(), raw)
			if err != nil {
				problem(w, r, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "the bearer token is invalid", false)
				return
			}
			if principal.ActorType != actorType || !principal.HasScope(requiredScope) || (actorType == "workload" && (principal.TenantID == "" || principal.ClientID == "")) {
				problem(w, r, http.StatusForbidden, "AUTHORIZATION_DENIED", "the authenticated principal lacks required authority", false)
				return
			}
			*r = *r.WithContext(auth.WithPrincipal(r.Context(), principal))
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) (string, bool) {
	scheme, value, ok := strings.Cut(strings.TrimSpace(header), " ")
	return value, ok && strings.EqualFold(scheme, "Bearer") && value != ""
}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (a *API) correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id != "" && !validUUID(id) {
			id = newUUID()
			w.Header().Set("X-Correlation-ID", id)
			r = r.WithContext(context.WithValue(r.Context(), correlationKey{}, id))
			problem(w, r, http.StatusBadRequest, "INVALID_CORRELATION_ID", "X-Correlation-ID must be a UUID", false)
			return
		}
		if id == "" {
			id = newUUID()
		}
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), correlationKey{}, id)))
	})
}

func (a *API) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)
		principal, _ := auth.PrincipalFromContext(r.Context())
		slog.InfoContext(r.Context(), "request completed", "method", r.Method, "path", r.URL.Path, "status", response.status, "actor_id", principal.Subject, "actor_type", principal.ActorType, "client_id", principal.ClientID, "tenant_id", principal.TenantID, "correlation_id", correlationID(r), "duration_ms", time.Since(started).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		problem(w, r, http.StatusServiceUnavailable, "SERVICE_NOT_READY", "database unavailable", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 16 || len(key) > 128 {
		problem(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 16 to 128 characters", false)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var command domain.RegisterTenant
	if err := dec.Decode(&command); err != nil {
		problem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is not valid contract JSON", false)
		return
	}
	// tenant_id is Control Plane-minted, never caller-supplied (see
	// domain.RegisterTenant and contracts/control-plane/v1/
	// tenant-registration.schema.json, which does not accept it as input).
	command.TenantID = domain.NewTenantID()
	if err := command.Validate(); err != nil {
		problem(w, r, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), false)
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	metadata := requestMetadata(r, principal)
	operation, err := a.store.RegisterTenant(r.Context(), key, metadata, command)
	if errors.Is(err, store.ErrIdempotencyConflict) {
		problem(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "the idempotency key was used for a different request", false)
		return
	}
	if err != nil {
		problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "tenant registration could not be persisted", true)
		return
	}
	w.Header().Set("Location", "/v1/operations/"+operation.OperationID)
	writeJSON(w, http.StatusAccepted, operation)
}

func (a *API) getTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if !domain.ValidTenantID(tenantID) {
		problem(w, r, 400, "invalid_tenant_id", "tenant_id is invalid", false)
		return
	}
	tenant, err := a.store.GetTenant(r.Context(), tenantID)
	if err != nil {
		var notFound domain.NotFoundError
		if errors.As(err, &notFound) {
			problem(w, r, 404, "tenant_not_found", err.Error(), false)
			return
		}
		problem(w, r, 500, "internal_error", "tenant lookup failed", true)
		return
	}
	writeJSON(w, 200, tenant)
}

func (a *API) getEntitlement(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenantId")
	productID := r.URL.Query().Get("productId")
	q := domain.EntitlementQuery{TenantID: tenantID, ProductID: productID}
	if err := q.Validate(); err != nil {
		problem(w, r, 422, "validation_failed", err.Error(), false)
		return
	}
	ent, err := a.store.GetEntitlement(r.Context(), tenantID, productID)
	if err != nil {
		var notFound domain.NotFoundError
		if errors.As(err, &notFound) {
			problem(w, r, 404, "entitlement_not_found", err.Error(), false)
			return
		}
		problem(w, r, 500, "internal_error", "entitlement lookup failed", true)
		return
	}
	writeJSON(w, 200, ent)
}

func (a *API) tenantLifecycleAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		cmd := domain.LifecycleAction{TenantID: tenantID, Action: action}
		if err := cmd.Validate(); err != nil {
			problem(w, r, 422, "validation_failed", err.Error(), false)
			return
		}
		var next domain.LifecycleStatus
		switch action {
		case "activate":
			next = domain.LifecycleActive
		case "suspend":
			next = domain.LifecycleSuspended
		case "decommission":
			next = domain.LifecycleDecommissioned
		}
		if err := a.store.UpdateTenantLifecycle(r.Context(), tenantID, next); err != nil {
			var notFound domain.NotFoundError
			if errors.As(err, &notFound) {
				problem(w, r, 404, "tenant_not_found", err.Error(), false)
				return
			}
			problem(w, r, 500, "internal_error", "tenant lifecycle update failed", true)
			return
		}
		writeJSON(w, 200, map[string]string{"tenant_id": tenantID, "status": string(next)})
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, r *http.Request, status int, code, detail string, retryable bool) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://docs.nabhold.com/problems/" + strings.ToLower(code), "title": http.StatusText(status), "status": status, "detail": detail, "code": code, "correlation_id": correlationID(r), "retryable": retryable})
}

func requestMetadata(r *http.Request, principal auth.Principal) store.RequestMetadata {
	return store.RequestMetadata{ActorID: principal.Subject, ActorType: principal.ActorType, ClientID: principal.ClientID, TokenID: principal.TokenID, CorrelationID: correlationID(r)}
}

func correlationID(r *http.Request) string {
	if value, ok := r.Context().Value(correlationKey{}).(string); ok {
		return value
	}
	return ""
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func newUUID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
