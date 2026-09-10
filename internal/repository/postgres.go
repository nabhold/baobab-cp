package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nabhold/baobab-cp/internal/domain"
	"github.com/nabhold/baobab-cp/internal/resolver"
)

// ErrMappingOverlap is returned when a mapping insert is rejected by the
// canonical_mapping_source_type_active_excl exclusion constraint (migration
// 000022): an active mapping of the same type already exists for the source
// entity with an overlapping validity period. Per the Canonical Mapping
// Model §67.5, an ambiguous authoritative mapping must fail explicitly
// rather than silently select one of the candidates.
var ErrMappingOverlap = errors.New("overlapping active mapping")

// ErrExternalIdentityAlreadyLinked is returned when LinkExternalIdentity's
// insert is rejected by identity.external_identity's UNIQUE(issuer, subject)
// constraint (migration 000026) -- ADR-0004 §7/§54's core invariant that a
// given external provider subject resolves to at most one Canonical
// Identity. Gate IAM-3 phase 3's provisioning flow (ADR-0004 §56, "Identity
// Provisioning Race") is expected to treat this as "someone else just
// created it concurrently" and re-resolve, not as a fatal error.
var ErrExternalIdentityAlreadyLinked = errors.New("external identity already linked to a principal")

// PostgresRepository is the PostgreSQL-backed repository implementation for mapping, capability, and topology data.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

var _ MappingRepository = (*PostgresRepository)(nil)
var _ CapabilityRepository = (*PostgresRepository)(nil)
var _ ResolverRepository = (*PostgresRepository)(nil)
var _ MappingWriter = (*PostgresRepository)(nil)
var _ CapabilityWriter = (*PostgresRepository)(nil)
var _ CanonicalEntityRepository = (*PostgresRepository)(nil)
var _ MappingScopeWriter = (*PostgresRepository)(nil)
var _ IdentityRepository = (*PostgresRepository)(nil)
var _ IdentityReferenceRepository = (*PostgresRepository)(nil)
var _ IdentityLinkingRepository = (*PostgresRepository)(nil)

func Open(ctx context.Context, url string) (*PostgresRepository, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}

func (r *PostgresRepository) CreateCanonicalEntity(ctx context.Context, entity domain.CanonicalEntity) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := entity.Validate(); err != nil {
		return fmt.Errorf("validate canonical entity: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO registry.canonical_entity(canonical_entity_id, tenant_id, legal_entity_id, entity_type, external_key, status)
		VALUES ($1::uuid, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), LOWER($6))`, entity.ID, entity.OwnerTenantID, entity.OwnerLegalEntityID, entity.EntityType, entity.CanonicalKey, entity.Status)
	return err
}

func (r *PostgresRepository) GetCanonicalEntity(ctx context.Context, id string) (domain.CanonicalEntity, error) {
	if r == nil || r.pool == nil {
		return domain.CanonicalEntity{}, errors.New("repository is not initialized")
	}
	var entity domain.CanonicalEntity
	var status string
	err := r.pool.QueryRow(ctx, `SELECT canonical_entity_id::text, COALESCE(tenant_id,''), COALESCE(legal_entity_id,''), entity_type, COALESCE(external_key,''), UPPER(status), version, created_at, updated_at FROM registry.canonical_entity WHERE canonical_entity_id=$1::uuid`, id).Scan(&entity.ID, &entity.OwnerTenantID, &entity.OwnerLegalEntityID, &entity.EntityType, &entity.CanonicalKey, &status, &entity.Version, &entity.CreatedAt, &entity.UpdatedAt)
	if err != nil {
		return domain.CanonicalEntity{}, fmt.Errorf("get canonical entity %s: %w", id, err)
	}
	entity.Status, entity.SchemaVersion, entity.Authority, entity.Classification = status, 1, "baobab", "INTERNAL"
	entity.DisplayName = entity.CanonicalKey
	entity.EffectiveFrom = entity.CreatedAt
	return entity, nil
}

func (r *PostgresRepository) SaveCanonicalEntity(ctx context.Context, entity domain.CanonicalEntity, expectedVersion int64) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	result, err := r.pool.Exec(ctx, `UPDATE registry.canonical_entity SET entity_type=$2, external_key=NULLIF($3,''), status=LOWER($4), version=version+1, updated_at=now() WHERE canonical_entity_id=$1::uuid AND version=$5`, entity.ID, entity.EntityType, entity.CanonicalKey, entity.Status, expectedVersion)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("canonical entity %s version conflict or not found", entity.ID)
	}
	return nil
}

func (r *PostgresRepository) ListMappings(ctx context.Context, canonicalEntityID string) ([]domain.Mapping, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cm.canonical_mapping_id::text, cm.mapping_type, COALESCE(ce.tenant_id, ''),
		       cm.source_entity_id::text, cm.target_entity_id::text,
		       COALESCE(ms.mapping_scope_id::text, cm.source_entity_id::text),
		       UPPER(cm.status), cm.effective_from, cm.effective_to, cm.created_at
		FROM mapping.canonical_mapping cm
		JOIN registry.canonical_entity ce ON ce.canonical_entity_id = cm.source_entity_id
		LEFT JOIN mapping.mapping_scope ms ON ms.tenant_id = ce.tenant_id
		WHERE ce.tenant_id = $1 OR cm.source_entity_id::text = $1
		ORDER BY cm.created_at DESC`, canonicalEntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Mapping
	for rows.Next() {
		var m domain.Mapping
		var effectiveFrom, createdAt time.Time
		var effectiveTo *time.Time
		if err := rows.Scan(
			&m.ID,
			&m.MappingType,
			&m.TenantID,
			&m.CanonicalEntityID,
			&m.TargetCanonicalEntityID,
			&m.ScopeID,
			&m.Status,
			&effectiveFrom,
			&effectiveTo,
			&createdAt,
		); err != nil {
			return nil, err
		}
		// Direction/Cardinality/Authority/Confidence/ResolutionPriority/Revision
		// have no backing columns on mapping.canonical_mapping yet -- see the
		// Mapping struct's own doc comment (domain/canonical.go) and migration
		// 000022's comment for the same pre-existing gap.
		m.Direction = "SOURCE_TO_TARGET"
		m.Cardinality = "ONE_TO_ONE"
		m.Authority = "baobab"
		m.Confidence = "CONFIRMED"
		m.ResolutionPriority = 0
		m.Revision = 1
		m.EffectiveFrom = effectiveFrom.UTC().Format(time.RFC3339)
		if effectiveTo != nil {
			m.EffectiveTo = effectiveTo.UTC().Format(time.RFC3339)
		}
		m.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) CreateMapping(ctx context.Context, mapping domain.Mapping) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("validate mapping: %w", err)
	}
	if mapping.ExternalReferenceID != "" {
		return errors.New("external-reference mappings are not supported by the current canonical schema")
	}
	// mapping.Validate() has already confirmed these parse as RFC3339; the
	// canonical_mapping_source_type_active_excl exclusion constraint (see
	// migration 000022) is what actually enforces non-overlap - this insert
	// simply must not silently discard the values, as it previously did.
	effectiveFrom, err := time.Parse(time.RFC3339, mapping.EffectiveFrom)
	if err != nil {
		return fmt.Errorf("parse effective_from: %w", err)
	}
	var effectiveTo *time.Time
	if mapping.EffectiveTo != "" {
		parsed, err := time.Parse(time.RFC3339, mapping.EffectiveTo)
		if err != nil {
			return fmt.Errorf("parse effective_to: %w", err)
		}
		effectiveTo = &parsed
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO mapping.canonical_mapping(canonical_mapping_id, source_entity_id, target_entity_id, mapping_type, status, effective_from, effective_to)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, LOWER($5), $6, $7)`, mapping.ID, mapping.CanonicalEntityID, mapping.TargetCanonicalEntityID, mapping.MappingType, mapping.Status, effectiveFrom, effectiveTo)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return fmt.Errorf("%w: an active mapping of type %s already exists for this source entity in an overlapping period", ErrMappingOverlap, mapping.MappingType)
		}
		return err
	}
	return nil
}

func (r *PostgresRepository) GetMapping(ctx context.Context, mappingID string) (domain.Mapping, error) {
	if r == nil || r.pool == nil {
		return domain.Mapping{}, errors.New("repository is not initialized")
	}
	var mapping domain.Mapping
	var effectiveFrom, createdAt time.Time
	var effectiveTo *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT cm.canonical_mapping_id::text, cm.mapping_type, COALESCE(ce.tenant_id, ''),
		       cm.source_entity_id::text, cm.target_entity_id::text, cm.source_entity_id::text,
		       UPPER(cm.status), cm.effective_from, cm.effective_to, cm.created_at
		FROM mapping.canonical_mapping cm
		JOIN registry.canonical_entity ce ON ce.canonical_entity_id = cm.source_entity_id
		WHERE cm.canonical_mapping_id=$1::uuid`, mappingID).Scan(
		&mapping.ID, &mapping.MappingType, &mapping.TenantID,
		&mapping.CanonicalEntityID, &mapping.TargetCanonicalEntityID, &mapping.ScopeID,
		&mapping.Status, &effectiveFrom, &effectiveTo, &createdAt,
	)
	if err != nil {
		return domain.Mapping{}, fmt.Errorf("get mapping %s: %w", mappingID, err)
	}
	// See ListMappings' comment: these have no backing column yet.
	mapping.Direction, mapping.Cardinality = "SOURCE_TO_TARGET", "ONE_TO_ONE"
	mapping.Authority, mapping.Confidence, mapping.Revision = "baobab", "CONFIRMED", 1
	mapping.EffectiveFrom = effectiveFrom.UTC().Format(time.RFC3339)
	if effectiveTo != nil {
		mapping.EffectiveTo = effectiveTo.UTC().Format(time.RFC3339)
	}
	mapping.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return mapping, nil
}

func (r *PostgresRepository) SaveMapping(ctx context.Context, mapping domain.Mapping, expectedVersion int64) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("validate mapping: %w", err)
	}
	if expectedVersion != 1 {
		return fmt.Errorf("mapping %s version conflict: expected %d, got 1", mapping.ID, expectedVersion)
	}
	result, err := r.pool.Exec(ctx, `UPDATE mapping.canonical_mapping SET mapping_type=$2, status=LOWER($3) WHERE canonical_mapping_id=$1::uuid`, mapping.ID, mapping.MappingType, mapping.Status)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("mapping %s not found", mapping.ID)
	}
	return nil
}

// CreateMappingScope, GetMappingScope and ListMappingScopes are Gate 2's
// (docs/reconciliation/platform-resolution-spine-audit.md) first real
// Postgres-backed persistence for domain.MappingScope -- Gate 1 (#61) gave
// it Go fields matching nabhold/shared's wire schema, but nothing loaded or
// saved one against mapping.mapping_scope until migration
// 000025_mapping_scope_dimensions.sql completed that table's columns.
//
// market_id, digital_estate_id, digital_property_id, engine_id and
// engine_instance_id remain this database's own uuid identifiers here
// (cast to text), not the wire schema's slug pattern -- see 000025's
// comment for why that conversion isn't made unilaterally in this pass.
func (r *PostgresRepository) CreateMappingScope(ctx context.Context, scope domain.MappingScope) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := scope.Validate(); err != nil {
		return fmt.Errorf("validate mapping scope: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mapping.mapping_scope(
			mapping_scope_id, tenant_id, market_id, legal_entity_id,
			organisation_id, business_unit_id, operating_region_id, geographic_region_id,
			country_code, digital_estate_id, digital_property_id, channel_id,
			currency_code, locale, catalogue_id, customer_segment_id,
			engine_id, engine_instance_id, environment, deployment_region,
			include_countries, exclude_countries
		)
		VALUES (
			$1::uuid, $2, NULLIF($3, '')::uuid, NULLIF($4, ''),
			NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, '')::uuid, NULLIF($11, '')::uuid, NULLIF($12, ''),
			NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''),
			NULLIF($17, '')::uuid, NULLIF($18, '')::uuid, NULLIF($19, ''), NULLIF($20, ''),
			$21, $22
		)`,
		scope.ScopeID, scope.TenantID, scope.MarketID, scope.LegalEntityID,
		scope.OrganisationID, scope.BusinessUnitID, scope.OperatingRegionID, scope.GeographicRegionID,
		scope.Country, scope.EstateID, scope.DigitalPropertyID, scope.ChannelID,
		scope.Currency, scope.Locale, scope.CatalogueID, scope.CustomerSegmentID,
		scope.EngineID, scope.EngineInstanceID, scope.Environment, scope.DeploymentRegion,
		scope.IncludeCountries, scope.ExcludeCountries,
	)
	return err
}

const mappingScopeSelectColumns = `
	mapping_scope_id::text, tenant_id, COALESCE(market_id::text, ''), COALESCE(legal_entity_id, ''),
	COALESCE(organisation_id, ''), COALESCE(business_unit_id, ''), COALESCE(operating_region_id, ''), COALESCE(geographic_region_id, ''),
	COALESCE(country_code, ''), COALESCE(digital_estate_id::text, ''), COALESCE(digital_property_id::text, ''), COALESCE(channel_id, ''),
	COALESCE(currency_code, ''), COALESCE(locale, ''), COALESCE(catalogue_id, ''), COALESCE(customer_segment_id, ''),
	COALESCE(engine_id::text, ''), COALESCE(engine_instance_id::text, ''), COALESCE(environment, ''), COALESCE(deployment_region, ''),
	include_countries, exclude_countries, created_at, updated_at`

func scanMappingScope(row interface {
	Scan(dest ...any) error
}) (domain.MappingScope, error) {
	var s domain.MappingScope
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&s.ScopeID, &s.TenantID, &s.MarketID, &s.LegalEntityID,
		&s.OrganisationID, &s.BusinessUnitID, &s.OperatingRegionID, &s.GeographicRegionID,
		&s.Country, &s.EstateID, &s.DigitalPropertyID, &s.ChannelID,
		&s.Currency, &s.Locale, &s.CatalogueID, &s.CustomerSegmentID,
		&s.EngineID, &s.EngineInstanceID, &s.Environment, &s.DeploymentRegion,
		&s.IncludeCountries, &s.ExcludeCountries, &createdAt, &updatedAt,
	)
	if err != nil {
		return domain.MappingScope{}, err
	}
	s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	s.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return s, nil
}

func (r *PostgresRepository) GetMappingScope(ctx context.Context, scopeID string) (domain.MappingScope, error) {
	if r == nil || r.pool == nil {
		return domain.MappingScope{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `SELECT `+mappingScopeSelectColumns+` FROM mapping.mapping_scope WHERE mapping_scope_id=$1::uuid`, scopeID)
	scope, err := scanMappingScope(row)
	if err != nil {
		return domain.MappingScope{}, fmt.Errorf("get mapping scope %s: %w", scopeID, err)
	}
	return scope, nil
}

func (r *PostgresRepository) ListMappingScopes(ctx context.Context, tenantID string) ([]domain.MappingScope, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `SELECT `+mappingScopeSelectColumns+` FROM mapping.mapping_scope WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.MappingScope
	for rows.Next() {
		scope, err := scanMappingScope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveIdentity, CreateIdentity and LinkExternalIdentity are Gate IAM-3
// phase 2 (docs/governance/gate-iam-3-canonical-identity-scope.md): the
// first real Postgres-backed persistence for domain.Principal/
// domain.ExternalIdentity, against the identity.principal/
// identity.external_identity tables migration 000026 added.
func (r *PostgresRepository) ResolveIdentity(ctx context.Context, issuer, subject string) (domain.Principal, error) {
	if r == nil || r.pool == nil {
		return domain.Principal{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `
		SELECT p.principal_id::text, p.actor_type, p.status, p.created_at, p.updated_at
		FROM identity.principal p
		JOIN identity.external_identity e ON e.principal_id = p.principal_id
		WHERE e.issuer = $1 AND e.subject = $2`, issuer, subject)
	var principal domain.Principal
	err := row.Scan(&principal.ID, &principal.ActorType, &principal.Status, &principal.CreatedAt, &principal.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Principal{}, ErrIdentityNotFound
		}
		return domain.Principal{}, fmt.Errorf("resolve identity: %w", err)
	}
	return principal, nil
}

func (r *PostgresRepository) GetPrincipal(ctx context.Context, principalID string) (domain.Principal, error) {
	if r == nil || r.pool == nil {
		return domain.Principal{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `
		SELECT principal_id::text, actor_type, status, created_at, updated_at
		FROM identity.principal
		WHERE principal_id = $1::uuid`, principalID)
	var principal domain.Principal
	err := row.Scan(&principal.ID, &principal.ActorType, &principal.Status, &principal.CreatedAt, &principal.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Principal{}, ErrIdentityNotFound
		}
		return domain.Principal{}, fmt.Errorf("get principal: %w", err)
	}
	return principal, nil
}

func (r *PostgresRepository) CreateIdentity(ctx context.Context, principal domain.Principal) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := principal.Validate(); err != nil {
		return fmt.Errorf("validate principal: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.principal(principal_id, actor_type, status)
		VALUES ($1::uuid, $2, $3)`,
		principal.ID, principal.ActorType, principal.Status)
	return err
}

func (r *PostgresRepository) LinkExternalIdentity(ctx context.Context, external domain.ExternalIdentity) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := external.Validate(); err != nil {
		return fmt.Errorf("validate external identity: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.external_identity(external_identity_id, principal_id, issuer, subject, provider_type, status)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), $6)`,
		external.ID, external.PrincipalID, external.Issuer, external.Subject, external.ProviderType, external.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExternalIdentityAlreadyLinked
		}
		return err
	}
	return nil
}

// LinkExternalIdentityAudited implements IdentityLinkingRepository: it
// links a second ExternalIdentity to an already-existing Principal and
// writes an audit_events row in the same transaction (ADR-0004 §16), so a
// link can never be recorded without being audited or vice versa.
func (r *PostgresRepository) LinkExternalIdentityAudited(ctx context.Context, external domain.ExternalIdentity, actor AuditActor, reason string) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := external.Validate(); err != nil {
		return fmt.Errorf("validate external identity: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO identity.external_identity(external_identity_id, principal_id, issuer, subject, provider_type, status)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), $6)`,
		external.ID, external.PrincipalID, external.Issuer, external.Subject, external.ProviderType, external.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrExternalIdentityAlreadyLinked
		}
		return err
	}

	payload, err := json.Marshal(map[string]string{
		"external_identity_id": external.ID,
		"issuer":               external.Issuer,
		"subject":              external.Subject,
		"provider_type":        external.ProviderType,
	})
	if err != nil {
		return fmt.Errorf("marshal link audit payload: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_events(actor_id, actor_type, client_id, token_id, correlation_id, action, target, result, policy_decision, payload)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10)`,
		actor.ActorID, actor.ActorType, actor.ClientID, actor.TokenID, actor.CorrelationID,
		"identity.external_identity.linked", "principal:"+external.PrincipalID, "success", reason, payload); err != nil {
		return fmt.Errorf("write link audit record: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) CreateIdentityReference(ctx context.Context, reference domain.IdentityReference) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := reference.Validate(); err != nil {
		return fmt.Errorf("validate identity reference: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.identity_reference(identity_reference_id, principal_id, engine, engine_instance_id, external_type, external_id, status)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''), $5, $6, $7)`,
		reference.ID, reference.PrincipalID, reference.Engine, reference.EngineInstanceID, reference.ExternalType, reference.ExternalID, reference.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrIdentityReferenceAlreadyMapped
		}
		return err
	}
	return nil
}

func (r *PostgresRepository) ListIdentityReferences(ctx context.Context, principalID string) ([]domain.IdentityReference, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT identity_reference_id::text, principal_id::text, engine, COALESCE(engine_instance_id, ''), external_type, external_id, status, created_at
		FROM identity.identity_reference
		WHERE principal_id = $1::uuid`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.IdentityReference
	for rows.Next() {
		var reference domain.IdentityReference
		if err := rows.Scan(&reference.ID, &reference.PrincipalID, &reference.Engine, &reference.EngineInstanceID, &reference.ExternalType, &reference.ExternalID, &reference.Status, &reference.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) ResolveIdentityReference(ctx context.Context, engine, externalType, externalID string) (domain.IdentityReference, error) {
	if r == nil || r.pool == nil {
		return domain.IdentityReference{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `
		SELECT identity_reference_id::text, principal_id::text, engine, COALESCE(engine_instance_id, ''), external_type, external_id, status, created_at
		FROM identity.identity_reference
		WHERE engine = $1 AND external_type = $2 AND external_id = $3`, engine, externalType, externalID)
	var reference domain.IdentityReference
	err := row.Scan(&reference.ID, &reference.PrincipalID, &reference.Engine, &reference.EngineInstanceID, &reference.ExternalType, &reference.ExternalID, &reference.Status, &reference.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.IdentityReference{}, ErrIdentityReferenceNotFound
		}
		return domain.IdentityReference{}, fmt.Errorf("resolve identity reference: %w", err)
	}
	return reference, nil
}

func (r *PostgresRepository) ListBindings(ctx context.Context, capabilityKey string) ([]resolver.CapabilityBinding, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			cb.id::text,
			cap.code,
			ei.engine_id,
			ei.engine_instance_id,
			cb.binding_mode,
			cb.priority,
			UPPER(cb.status),
			cb.contract_version,
			cb.scope_id::text
		FROM capability.capability_binding cb
		JOIN capability.capability cap ON cap.capability_id = cb.capability_id
		JOIN topology.engine_instance ei ON ei.engine_instance_id = cb.engine_instance_id
		WHERE cap.code = $1
		ORDER BY cb.priority DESC, cb.binding_mode ASC`, capabilityKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []resolver.CapabilityBinding
	for rows.Next() {
		var b resolver.CapabilityBinding
		if err := rows.Scan(
			&b.ID,
			&b.CapabilityKey,
			&b.EngineID,
			&b.EngineInstanceID,
			&b.BindingMode,
			&b.Priority,
			&b.Status,
			&b.ContractVersion,
			&b.ScopeID,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) CreateBinding(ctx context.Context, binding resolver.CapabilityBinding) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if binding.CapabilityKey == "" || binding.EngineID == "" || binding.EngineInstanceID == "" || binding.ScopeID == "" {
		return errors.New("capability, engine, engine instance, and scope are required")
	}
	// status and binding_mode are stored upper-cased so that the
	// capability_binding_primary_excl exclusion constraint (which is defined
	// against status = 'ACTIVE' AND binding_mode = 'PRIMARY') actually fires.
	// This previously lower-cased status only, which silently defeated that
	// constraint for every binding created through this path: see
	// docs/adr/ADR-0005-bcp-db-001-conformance-gap.md.
	_, err := r.pool.Exec(ctx, `
		INSERT INTO capability.capability_binding(capability_id, engine_instance_id, scope_id, binding_mode, priority, status, contract_version, effective_from)
		SELECT c.capability_id, ei.engine_instance_id, $4::uuid, UPPER($5), $6, UPPER($7), $8, now()
		FROM capability.capability c JOIN topology.engine_instance ei ON ei.engine_instance_id=$3::uuid AND ei.engine_id=$2::uuid
		WHERE c.code=$1`, binding.CapabilityKey, binding.EngineID, binding.EngineInstanceID, binding.ScopeID, binding.BindingMode, binding.Priority, binding.Status, binding.ContractVersion)
	return err
}

func (r *PostgresRepository) SaveBinding(ctx context.Context, binding resolver.CapabilityBinding, expectedVersion int64) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	result, err := r.pool.Exec(ctx, `UPDATE capability.capability_binding cb SET binding_mode=UPPER($2), priority=$3, status=UPPER($4), contract_version=$5, version=version+1, updated_at=now() FROM capability.capability c WHERE cb.id=$1::uuid AND cb.capability_id=c.capability_id AND c.code=$6 AND cb.version=$7`, binding.ID, binding.BindingMode, binding.Priority, binding.Status, binding.ContractVersion, binding.CapabilityKey, expectedVersion)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("binding %s version conflict or not found", binding.EngineInstanceID)
	}
	return nil
}

func (r *PostgresRepository) ListActiveInstances(ctx context.Context, engineID string) ([]resolver.EngineInstance, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT engine_instance_id, engine_id, region, environment, status
		FROM topology.engine_instance
		WHERE engine_id = $1::uuid AND UPPER(status) = 'ACTIVE'
		ORDER BY region ASC`, engineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []resolver.EngineInstance
	for rows.Next() {
		var instance resolver.EngineInstance
		if err := rows.Scan(&instance.ID, &instance.EngineID, &instance.Region, &instance.Environment, &instance.Status); err != nil {
			return nil, err
		}
		out = append(out, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	return r.pool.Ping(ctx)
}
