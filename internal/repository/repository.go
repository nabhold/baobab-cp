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
	CreateIdentity(ctx context.Context, principal domain.Principal) error
	LinkExternalIdentity(ctx context.Context, external domain.ExternalIdentity) error
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
}

var _ MappingRepository = (*Repository)(nil)
var _ CapabilityRepository = (*Repository)(nil)
var _ ResolverRepository = (*Repository)(nil)
var _ MappingWriter = (*Repository)(nil)
var _ CapabilityWriter = (*Repository)(nil)
var _ MappingScopeWriter = (*Repository)(nil)
var _ IdentityRepository = (*Repository)(nil)
var _ IdentityReferenceRepository = (*Repository)(nil)

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
	if !ok {
		return domain.Principal{}, ErrIdentityNotFound
	}
	principal, ok := r.Principals[external.PrincipalID]
	if !ok {
		return domain.Principal{}, fmt.Errorf("external identity %s references missing principal %s", external.ID, external.PrincipalID)
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
