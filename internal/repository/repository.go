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

// Repository is a lightweight in-memory repository backing the resolver/service layer.
type Repository struct {
	Mappings        map[string][]domain.Mapping
	Bindings        map[string][]resolver.CapabilityBinding
	EngineInstances map[string][]resolver.EngineInstance
	MappingScopes   map[string]domain.MappingScope // keyed by ScopeID
}

var _ MappingRepository = (*Repository)(nil)
var _ CapabilityRepository = (*Repository)(nil)
var _ ResolverRepository = (*Repository)(nil)
var _ MappingWriter = (*Repository)(nil)
var _ CapabilityWriter = (*Repository)(nil)
var _ MappingScopeWriter = (*Repository)(nil)

func NewInMemoryRepository() *Repository {
	return &Repository{
		Mappings:        map[string][]domain.Mapping{},
		Bindings:        map[string][]resolver.CapabilityBinding{},
		EngineInstances: map[string][]resolver.EngineInstance{},
		MappingScopes:   map[string]domain.MappingScope{},
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
