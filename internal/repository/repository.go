package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/resolver"
)

// MappingRepository defines the contract for retrieving canonical mappings.
type MappingRepository interface {
	ListMappings(ctx context.Context, canonicalEntityID string) ([]domain.Mapping, error)
}

// CapabilityRepository defines the contract for retrieving capability bindings and engine instances.
type CapabilityRepository interface {
	ListBindings(ctx context.Context, capabilityKey string) ([]resolver.CapabilityBinding, error)
	ListActiveInstances(ctx context.Context, engineID string) ([]resolver.EngineInstance, error)
}

// ResolverRepository combines the mapping and capability read contracts used by the resolution stack.
type ResolverRepository interface {
	MappingRepository
	CapabilityRepository
}

// MappingWriter is the mutable mapping contract implemented by repositories
// that support the current simplified mapping aggregate.
type MappingWriter interface {
	CreateMapping(ctx context.Context, mapping domain.Mapping) error
	GetMapping(ctx context.Context, mappingID string) (domain.Mapping, error)
	SaveMapping(ctx context.Context, mapping domain.Mapping, expectedVersion int64) error
}

// CapabilityWriter is the mutable capability-binding contract implemented by
// repositories that support the current simplified binding aggregate.
type CapabilityWriter interface {
	CreateBinding(ctx context.Context, binding resolver.CapabilityBinding) error
	SaveBinding(ctx context.Context, binding resolver.CapabilityBinding, expectedVersion int64) error
}

// MappingScopeWriter is the mutable MappingScope contract (Gate 2,
// docs/reconciliation/platform-resolution-spine-audit.md): the first
// repositories to support persisting the scope dimensions Gate 1 (#61)
// already gave domain.MappingScope Go fields for.
type MappingScopeWriter interface {
	CreateMappingScope(ctx context.Context, scope domain.MappingScope) error
	GetMappingScope(ctx context.Context, scopeID string) (domain.MappingScope, error)
	ListMappingScopes(ctx context.Context, tenantID string) ([]domain.MappingScope, error)
}

// ErrIdentityNotFound is returned by ResolveIdentity when no ExternalIdentity
// exists for the given (issuer, subject) pair -- ADR-0004 §11's "absent"
// branch of identity resolution, distinct from a repository failure, so
// Gate IAM-3 phase 3's first-authentication provisioning flow can branch on
// it via errors.Is instead of string-matching.
var ErrIdentityNotFound = errors.New("identity not found")

// IdentityRepository is the Gate IAM-3 contract
// (docs/governance/gate-iam-3-canonical-identity-scope.md, phase 2) for
// ADR-0004's CanonicalIdentity/ExternalIdentity(issuer, subject) layer.
// ResolveIdentity is the read side of ADR-0004 §11's resolution flow;
// CreateIdentity and LinkExternalIdentity are the write side a
// first-authentication provisioning flow (phase 3, ADR-0004 §12/§56) calls
// when ResolveIdentity returns ErrIdentityNotFound. Callers are expected to
// mint IDs via domain.NewPrincipalID()/NewExternalIdentityID() before
// calling, matching every other Create* method in this package.
type IdentityRepository interface {
	ResolveIdentity(ctx context.Context, issuer, subject string) (domain.Principal, error)
	// GetPrincipal returns the Principal identified by principalID.
	// ADR-0004 §15-22's linking/unlinking/merging flows (Gate IAM-3 phase
	// 6) all operate on an already-resolved Principal by ID, unlike
	// ResolveIdentity's (issuer, subject) lookup. Returns
	// ErrIdentityNotFound if no such Principal exists.
	GetPrincipal(ctx context.Context, principalID string) (domain.Principal, error)
	CreateIdentity(ctx context.Context, principal domain.Principal) error
	LinkExternalIdentity(ctx context.Context, external domain.ExternalIdentity) error
}

// AuditActor identifies who performed a security-sensitive identity
// operation (linking, unlinking, merging), for the audit record ADR-0004
// §16/§21/§52 require alongside it. Mirrors internal/store's
// RequestMetadata shape -- callers building either already have this same
// set of fields off a verified auth.Principal -- but is defined locally
// rather than imported, to keep internal/repository (mapping/capability
// /identity) and internal/store (tenant/context) independent of each
// other's packages.
type AuditActor struct {
	ActorID       string
	ActorType     string
	ClientID      string
	TokenID       string
	CorrelationID string
}

// IdentityLinkingRepository is the Gate IAM-3 phase 6 contract
// (docs/governance/gate-iam-3-canonical-identity-scope.md) for ADR-0004
// §15-17's identity-linking flow: adding a second ExternalIdentity to an
// already-existing Principal. Unlike LinkExternalIdentity (phase 2, used by
// phase 3's first-authentication provisioning, which has no existing
// Principal to audit a link *against* yet), this SHALL always write an
// audit record alongside the link -- ADR-0004 §16, "Every successful link
// SHALL be auditable." Callers are responsible for having already
// established both factors §15 requires (an existing authenticated session
// for the target Principal, and a fresh authentication with the new
// provider yielding the (issuer, subject) being linked); this method does
// not itself re-verify either -- it is the audited persistence step, not
// the authentication protocol.
type IdentityLinkingRepository interface {
	LinkExternalIdentityAudited(ctx context.Context, external domain.ExternalIdentity, actor AuditActor, reason string) error
}

// ErrExternalIdentityNotLinked is returned by UnlinkExternalIdentityAudited
// when the requested (issuer, subject) is not an ACTIVE ExternalIdentity of
// the given Principal -- it doesn't exist, belongs to a different
// Principal, or has already been unlinked/disabled/revoked.
var ErrExternalIdentityNotLinked = errors.New("external identity is not an active credential of this principal")

// ErrLastCredentialDenied is returned when unlinking would leave a
// Principal with no remaining ACTIVE ExternalIdentity and the caller did
// not mark the operation administrative -- ADR-0004 §18: "Unlinking SHALL
// verify that the actor will not be left without a valid authentication
// path unless the operation is explicitly administrative."
var ErrLastCredentialDenied = errors.New("unlinking this identity would leave the principal without any active credential")

// IdentityUnlinkingRepository is the Gate IAM-3 phase 6 contract for
// ADR-0004 §18's identity-unlinking flow. UnlinkExternalIdentityAudited
// marks the (issuer, subject) ExternalIdentity belonging to principalID as
// UNLINKED (never deleted -- ADR-0004 §34/§21's "retired identity SHALL
// not simply disappear" spirit, though written for canonical identities,
// applies just as well to a single credential's history) rather than
// deleting the row. Denies the operation with ErrLastCredentialDenied
// unless administrative is true and the Principal would otherwise be left
// with zero ACTIVE ExternalIdentities -- callers set administrative for
// explicit administrative recovery flows (§18's own example: "establishing
// another credential first; administrative recovery; explicit account
// disablement"). Like LinkExternalIdentityAudited, always writes an
// audit_events row in the same transaction (ADR-0004 §52).
type IdentityUnlinkingRepository interface {
	UnlinkExternalIdentityAudited(ctx context.Context, principalID, issuer, subject string, administrative bool, actor AuditActor, reason string) error
}

// ErrIdentityReferenceNotFound is returned by ResolveIdentityReference when
// no mapping exists for the given engine-native actor -- the "absent"
// branch of ADR-0004 §23-27's engine-reference resolution, mirroring
// ErrIdentityNotFound's role for issuer/subject resolution.
var ErrIdentityReferenceNotFound = errors.New("identity reference not found")

// ErrIdentityReferenceAlreadyMapped is returned when CreateIdentityReference
// is rejected by identity.identity_reference's UNIQUE(engine, external_type,
// external_id) constraint (migration 000027) -- ADR-0004 §29's collision
// -avoidance invariant: a given engine-native actor resolves to at most one
// Principal.
var ErrIdentityReferenceAlreadyMapped = errors.New("engine-native actor is already mapped to a principal")

// IdentityReferenceRepository is the Gate IAM-3 phase 5 contract
// (docs/governance/gate-iam-3-canonical-identity-scope.md) for ADR-0004
// §23-30's engine-reference mapping: a Principal to an engine-native actor
// (Medusa customer, iDempiere AD_User, Payload user, ...) and back.
type IdentityReferenceRepository interface {
	CreateIdentityReference(ctx context.Context, reference domain.IdentityReference) error
	ListIdentityReferences(ctx context.Context, principalID string) ([]domain.IdentityReference, error)
	ResolveIdentityReference(ctx context.Context, engine, externalType, externalID string) (domain.IdentityReference, error)
}

// Repository is a lightweight in-memory repository backing the resolver/service layer.
type Repository struct {
	Mappings           map[string][]domain.Mapping
	Bindings           map[string][]resolver.CapabilityBinding
	EngineInstances    map[string][]resolver.EngineInstance
	MappingScopes      map[string]domain.MappingScope      // keyed by ScopeID
	Principals         map[string]domain.Principal         // keyed by ID
	ExternalIdentities map[string]domain.ExternalIdentity  // keyed by "issuer\x00subject", mirroring UNIQUE(issuer, subject)
	IdentityReferences map[string]domain.IdentityReference // keyed by "engine\x00external_type\x00external_id", mirroring UNIQUE(engine, external_type, external_id)
	// LinkAudit records every LinkExternalIdentityAudited call, purely in
	// memory (there is no real audit_events table to write to here) so
	// tests can assert an audit entry was actually produced.
	LinkAudit []LinkAuditRecord
	// UnlinkAudit is LinkAudit's counterpart for UnlinkExternalIdentityAudited.
	UnlinkAudit []UnlinkAuditRecord
}

// LinkAuditRecord is the in-memory equivalent of the audit_events row
// PostgresRepository.LinkExternalIdentityAudited writes.
type LinkAuditRecord struct {
	External domain.ExternalIdentity
	Actor    AuditActor
	Reason   string
}

// UnlinkAuditRecord is the in-memory equivalent of the audit_events row
// PostgresRepository.UnlinkExternalIdentityAudited writes.
type UnlinkAuditRecord struct {
	PrincipalID    string
	Issuer         string
	Subject        string
	Administrative bool
	Actor          AuditActor
	Reason         string
}

var _ MappingRepository = (*Repository)(nil)
var _ CapabilityRepository = (*Repository)(nil)
var _ ResolverRepository = (*Repository)(nil)
var _ MappingWriter = (*Repository)(nil)
var _ CapabilityWriter = (*Repository)(nil)
var _ MappingScopeWriter = (*Repository)(nil)
var _ IdentityRepository = (*Repository)(nil)
var _ IdentityReferenceRepository = (*Repository)(nil)
var _ IdentityLinkingRepository = (*Repository)(nil)
var _ IdentityUnlinkingRepository = (*Repository)(nil)

func NewInMemoryRepository() *Repository {
	return &Repository{
		Mappings:           map[string][]domain.Mapping{},
		Bindings:           map[string][]resolver.CapabilityBinding{},
		EngineInstances:    map[string][]resolver.EngineInstance{},
		MappingScopes:      map[string]domain.MappingScope{},
		Principals:         map[string]domain.Principal{},
		ExternalIdentities: map[string]domain.ExternalIdentity{},
		IdentityReferences: map[string]domain.IdentityReference{},
	}
}

func (r *Repository) ListMappings(_ context.Context, canonicalEntityID string) ([]domain.Mapping, error) {
	if r == nil {
		return nil, errors.New("repository is nil")
	}
	mappings, ok := r.Mappings[canonicalEntityID]
	if !ok {
		return nil, fmt.Errorf("no mappings for %s", canonicalEntityID)
	}
	return mappings, nil
}

func (r *Repository) CreateMapping(_ context.Context, mapping domain.Mapping) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("validate mapping: %w", err)
	}
	if mapping.ID == "" {
		return errors.New("mapping id is required")
	}
	if _, err := r.GetMapping(context.Background(), mapping.ID); err == nil {
		return fmt.Errorf("mapping %s already exists", mapping.ID)
	}
	mapping.Revision = 1
	r.Mappings[mapping.CanonicalEntityID] = append(r.Mappings[mapping.CanonicalEntityID], mapping)
	return nil
}

func (r *Repository) GetMapping(_ context.Context, mappingID string) (domain.Mapping, error) {
	if r == nil {
		return domain.Mapping{}, errors.New("repository is nil")
	}
	for _, mappings := range r.Mappings {
		for _, mapping := range mappings {
			if mapping.ID == mappingID {
				return mapping, nil
			}
		}
	}
	return domain.Mapping{}, fmt.Errorf("mapping %s not found", mappingID)
}

func (r *Repository) SaveMapping(_ context.Context, mapping domain.Mapping, expectedVersion int64) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("validate mapping: %w", err)
	}
	for entityID, mappings := range r.Mappings {
		for index, current := range mappings {
			if current.ID != mapping.ID {
				continue
			}
			if current.Revision != expectedVersion {
				return fmt.Errorf("mapping %s version conflict: expected %d, got %d", mapping.ID, expectedVersion, current.Revision)
			}
			mapping.Revision = current.Revision + 1
			r.Mappings[entityID][index] = mapping
			return nil
		}
	}
	return fmt.Errorf("mapping %s not found", mapping.ID)
}

func (r *Repository) CreateMappingScope(_ context.Context, scope domain.MappingScope) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := scope.Validate(); err != nil {
		return fmt.Errorf("validate mapping scope: %w", err)
	}
	if _, exists := r.MappingScopes[scope.ScopeID]; exists {
		return fmt.Errorf("mapping scope %s already exists", scope.ScopeID)
	}
	r.MappingScopes[scope.ScopeID] = scope
	return nil
}

func (r *Repository) GetMappingScope(_ context.Context, scopeID string) (domain.MappingScope, error) {
	if r == nil {
		return domain.MappingScope{}, errors.New("repository is nil")
	}
	scope, ok := r.MappingScopes[scopeID]
	if !ok {
		return domain.MappingScope{}, fmt.Errorf("mapping scope %s not found", scopeID)
	}
	return scope, nil
}

func (r *Repository) ListMappingScopes(_ context.Context, tenantID string) ([]domain.MappingScope, error) {
	if r == nil {
		return nil, errors.New("repository is nil")
	}
	var out []domain.MappingScope
	for _, scope := range r.MappingScopes {
		if scope.TenantID == tenantID {
			out = append(out, scope)
		}
	}
	return out, nil
}

func externalIdentityKey(issuer, subject string) string { return issuer + "\x00" + subject }

func (r *Repository) ResolveIdentity(_ context.Context, issuer, subject string) (domain.Principal, error) {
	if r == nil {
		return domain.Principal{}, errors.New("repository is nil")
	}
	external, ok := r.ExternalIdentities[externalIdentityKey(issuer, subject)]
	if !ok || external.Status != "ACTIVE" {
		// ADR-0004 §32: external identities may independently be UNLINKED,
		// DISABLED or REVOKED -- such a row must not resolve, otherwise
		// UnlinkExternalIdentityAudited (Gate IAM-3 phase 6) would leave a
		// still-usable authentication path behind it.
		return domain.Principal{}, ErrIdentityNotFound
	}
	principal, ok := r.Principals[external.PrincipalID]
	if !ok {
		return domain.Principal{}, fmt.Errorf("external identity %s references missing principal %s", external.ID, external.PrincipalID)
	}
	return principal, nil
}

func (r *Repository) GetPrincipal(_ context.Context, principalID string) (domain.Principal, error) {
	if r == nil {
		return domain.Principal{}, errors.New("repository is nil")
	}
	principal, ok := r.Principals[principalID]
	if !ok {
		return domain.Principal{}, ErrIdentityNotFound
	}
	return principal, nil
}

func (r *Repository) CreateIdentity(_ context.Context, principal domain.Principal) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := principal.Validate(); err != nil {
		return fmt.Errorf("validate principal: %w", err)
	}
	if principal.ID == "" {
		return errors.New("principal id is required")
	}
	if _, exists := r.Principals[principal.ID]; exists {
		return fmt.Errorf("principal %s already exists", principal.ID)
	}
	r.Principals[principal.ID] = principal
	return nil
}

func (r *Repository) LinkExternalIdentity(_ context.Context, external domain.ExternalIdentity) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := external.Validate(); err != nil {
		return fmt.Errorf("validate external identity: %w", err)
	}
	if external.ID == "" {
		return errors.New("external identity id is required")
	}
	if _, exists := r.Principals[external.PrincipalID]; !exists {
		return fmt.Errorf("principal %s does not exist", external.PrincipalID)
	}
	key := externalIdentityKey(external.Issuer, external.Subject)
	if _, exists := r.ExternalIdentities[key]; exists {
		return fmt.Errorf("issuer %s subject already linked to a principal", external.Issuer)
	}
	r.ExternalIdentities[key] = external
	return nil
}

func (r *Repository) LinkExternalIdentityAudited(ctx context.Context, external domain.ExternalIdentity, actor AuditActor, reason string) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := r.LinkExternalIdentity(ctx, external); err != nil {
		return err
	}
	r.LinkAudit = append(r.LinkAudit, LinkAuditRecord{External: external, Actor: actor, Reason: reason})
	return nil
}

func (r *Repository) UnlinkExternalIdentityAudited(_ context.Context, principalID, issuer, subject string, administrative bool, actor AuditActor, reason string) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	key := externalIdentityKey(issuer, subject)
	external, ok := r.ExternalIdentities[key]
	if !ok || external.PrincipalID != principalID || external.Status != "ACTIVE" {
		return ErrExternalIdentityNotLinked
	}
	if !administrative {
		activeCount := 0
		for _, e := range r.ExternalIdentities {
			if e.PrincipalID == principalID && e.Status == "ACTIVE" {
				activeCount++
			}
		}
		if activeCount <= 1 {
			return ErrLastCredentialDenied
		}
	}
	external.Status = "UNLINKED"
	r.ExternalIdentities[key] = external
	r.UnlinkAudit = append(r.UnlinkAudit, UnlinkAuditRecord{
		PrincipalID: principalID, Issuer: issuer, Subject: subject,
		Administrative: administrative, Actor: actor, Reason: reason,
	})
	return nil
}

func identityReferenceKey(engine, externalType, externalID string) string {
	return engine + "\x00" + externalType + "\x00" + externalID
}

func (r *Repository) CreateIdentityReference(_ context.Context, reference domain.IdentityReference) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if err := reference.Validate(); err != nil {
		return fmt.Errorf("validate identity reference: %w", err)
	}
	if reference.ID == "" {
		return errors.New("identity reference id is required")
	}
	if _, exists := r.Principals[reference.PrincipalID]; !exists {
		return fmt.Errorf("principal %s does not exist", reference.PrincipalID)
	}
	key := identityReferenceKey(reference.Engine, reference.ExternalType, reference.ExternalID)
	if _, exists := r.IdentityReferences[key]; exists {
		return ErrIdentityReferenceAlreadyMapped
	}
	r.IdentityReferences[key] = reference
	return nil
}

func (r *Repository) ListIdentityReferences(_ context.Context, principalID string) ([]domain.IdentityReference, error) {
	if r == nil {
		return nil, errors.New("repository is nil")
	}
	var out []domain.IdentityReference
	for _, reference := range r.IdentityReferences {
		if reference.PrincipalID == principalID {
			out = append(out, reference)
		}
	}
	return out, nil
}

func (r *Repository) ResolveIdentityReference(_ context.Context, engine, externalType, externalID string) (domain.IdentityReference, error) {
	if r == nil {
		return domain.IdentityReference{}, errors.New("repository is nil")
	}
	reference, ok := r.IdentityReferences[identityReferenceKey(engine, externalType, externalID)]
	if !ok {
		return domain.IdentityReference{}, ErrIdentityReferenceNotFound
	}
	return reference, nil
}

func (r *Repository) ListBindings(_ context.Context, capabilityKey string) ([]resolver.CapabilityBinding, error) {
	if r == nil {
		return nil, errors.New("repository is nil")
	}
	bindings, ok := r.Bindings[capabilityKey]
	if !ok {
		return nil, fmt.Errorf("no bindings for %s", capabilityKey)
	}
	return bindings, nil
}

func (r *Repository) CreateBinding(_ context.Context, binding resolver.CapabilityBinding) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	if binding.CapabilityKey == "" {
		return errors.New("capability key is required")
	}
	if binding.EngineID == "" || binding.EngineInstanceID == "" {
		return errors.New("engine and engine instance are required")
	}
	for _, existing := range r.Bindings[binding.CapabilityKey] {
		if existing.EngineInstanceID == binding.EngineInstanceID {
			return fmt.Errorf("binding for engine instance %s already exists", binding.EngineInstanceID)
		}
	}
	binding.Version = 1
	r.Bindings[binding.CapabilityKey] = append(r.Bindings[binding.CapabilityKey], binding)
	return nil
}

func (r *Repository) SaveBinding(_ context.Context, binding resolver.CapabilityBinding, expectedVersion int64) error {
	if r == nil {
		return errors.New("repository is nil")
	}
	for index, current := range r.Bindings[binding.CapabilityKey] {
		if current.EngineInstanceID == binding.EngineInstanceID {
			if current.Version != expectedVersion {
				return fmt.Errorf("binding for engine instance %s version conflict: expected %d, got %d", binding.EngineInstanceID, expectedVersion, current.Version)
			}
			binding.Version = current.Version + 1
			r.Bindings[binding.CapabilityKey][index] = binding
			return nil
		}
	}
	return fmt.Errorf("binding for engine instance %s not found", binding.EngineInstanceID)
}

func (r *Repository) ListActiveInstances(_ context.Context, engineID string) ([]resolver.EngineInstance, error) {
	if r == nil {
		return nil, errors.New("repository is nil")
	}
	instances, ok := r.EngineInstances[engineID]
	if !ok {
		return nil, fmt.Errorf("no engine instances for %s", engineID)
	}
	active := make([]resolver.EngineInstance, 0, len(instances))
	for _, instance := range instances {
		if instance.Status == "ACTIVE" {
			active = append(active, instance)
		}
	}
	return active, nil
}
