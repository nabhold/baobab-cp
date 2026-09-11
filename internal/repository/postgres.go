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
	capabilitydomain "github.com/nabhold/baobab-cp/internal/capability/domain"
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
var _ CapabilityScopeWriter = (*PostgresRepository)(nil)
var _ CapabilityGrantRepository = (*PostgresRepository)(nil)
var _ CapabilityGrantWriter = (*PostgresRepository)(nil)
var _ CapabilityRegistryRepository = (*PostgresRepository)(nil)
var _ CapabilityRegistryWriter = (*PostgresRepository)(nil)
var _ IdentityRepository = (*PostgresRepository)(nil)
var _ IdentityReferenceRepository = (*PostgresRepository)(nil)
var _ IdentityLinkingRepository = (*PostgresRepository)(nil)
var _ IdentityUnlinkingRepository = (*PostgresRepository)(nil)
var _ IdentityMergeRepository = (*PostgresRepository)(nil)
var _ ContextRepository = (*PostgresRepository)(nil)
var _ ContextWriter = (*PostgresRepository)(nil)
var _ ContextStore = (*PostgresRepository)(nil)

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
	// ADR-0004 §32: external identities may independently be UNLINKED,
	// DISABLED or REVOKED -- such a row must not resolve, otherwise
	// UnlinkExternalIdentityAudited (Gate IAM-3 phase 6) would leave a
	// still-usable authentication path behind it.
	row := r.pool.QueryRow(ctx, `
		SELECT p.principal_id::text, p.actor_type, p.status, p.created_at, p.updated_at
		FROM identity.principal p
		JOIN identity.external_identity e ON e.principal_id = p.principal_id
		WHERE e.issuer = $1 AND e.subject = $2 AND e.status = 'ACTIVE'`, issuer, subject)
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
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, '')::uuid, $6, $7, $8, $9, $10)`,
		actor.ActorID, actor.ActorType, actor.ClientID, actor.TokenID, actor.CorrelationID,
		"identity.external_identity.linked", "principal:"+external.PrincipalID, "success", reason, payload); err != nil {
		return fmt.Errorf("write link audit record: %w", err)
	}

	return tx.Commit(ctx)
}

// UnlinkExternalIdentityAudited implements IdentityUnlinkingRepository
// (ADR-0004 §18). Unless administrative is true, it locks (SELECT ... FOR
// UPDATE) and counts the Principal's currently-ACTIVE ExternalIdentity rows
// within the same transaction before deciding, closing the race a plain
// COUNT(*) would leave between "how many active credentials remain" and
// the UPDATE that removes one -- ADR-0004 §17 calls this whole area "an
// account-takeover boundary", so this guard is worth the extra row lock.
// (Postgres rejects FOR UPDATE combined with an aggregate directly, hence
// counting the locked rows in Go rather than via SELECT count(*) ... FOR
// UPDATE.)
func (r *PostgresRepository) UnlinkExternalIdentityAudited(ctx context.Context, principalID, issuer, subject string, administrative bool, actor AuditActor, reason string) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if !administrative {
		rows, err := tx.Query(ctx, `
			SELECT external_identity_id FROM identity.external_identity
			WHERE principal_id = $1::uuid AND status = 'ACTIVE'
			FOR UPDATE`, principalID)
		if err != nil {
			return fmt.Errorf("lock active external identities: %w", err)
		}
		activeCount := 0
		for rows.Next() {
			activeCount++
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return fmt.Errorf("lock active external identities: %w", rowsErr)
		}
		// activeCount includes the row about to be unlinked, so <=1 means
		// this unlink would leave zero remaining.
		if activeCount <= 1 {
			return ErrLastCredentialDenied
		}
	}

	result, err := tx.Exec(ctx, `
		UPDATE identity.external_identity
		SET status = 'UNLINKED'
		WHERE principal_id = $1::uuid AND issuer = $2 AND subject = $3 AND status = 'ACTIVE'`,
		principalID, issuer, subject)
	if err != nil {
		return fmt.Errorf("unlink external identity: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrExternalIdentityNotLinked
	}

	payload, err := json.Marshal(map[string]any{
		"issuer":         issuer,
		"subject":        subject,
		"administrative": administrative,
	})
	if err != nil {
		return fmt.Errorf("marshal unlink audit payload: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_events(actor_id, actor_type, client_id, token_id, correlation_id, action, target, result, policy_decision, payload)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, '')::uuid, $6, $7, $8, $9, $10)`,
		actor.ActorID, actor.ActorType, actor.ClientID, actor.TokenID, actor.CorrelationID,
		"identity.external_identity.unlinked", "principal:"+principalID, "success", reason, payload); err != nil {
		return fmt.Errorf("write unlink audit record: %w", err)
	}

	return tx.Commit(ctx)
}

// MergePrincipalsAudited implements IdentityMergeRepository (ADR-0004
// §19-21). It locks both Principal rows (SELECT ... FOR UPDATE, ordered by
// ID to avoid deadlocking against a concurrent merge in the opposite
// direction), transfers every external_identity and identity_reference row
// from source to target via UPDATE ... RETURNING (safe from collision:
// neither UNIQUE(issuer, subject) nor UNIQUE(engine, external_type,
// external_id) includes principal_id), archives the source, and writes two
// audit_events rows -- one targeting the source, one targeting the target,
// sharing actor.CorrelationID -- so either Principal's own audit trail
// reveals the merge (ADR-0004 §21 requires preserving both "source
// identity" and "target identity" as auditable facts).
func (r *PostgresRepository) MergePrincipalsAudited(ctx context.Context, sourcePrincipalID, targetPrincipalID string, actor AuditActor, reason string) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if sourcePrincipalID == "" || targetPrincipalID == "" {
		return errors.New("source and target principal ids are required")
	}
	if sourcePrincipalID == targetPrincipalID {
		return ErrMergeSourceEqualsTarget
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock in a fixed order (by ID) regardless of which is source/target,
	// so two concurrent merges naming the same pair in opposite directions
	// can't deadlock against each other.
	first, second := sourcePrincipalID, targetPrincipalID
	if second < first {
		first, second = second, first
	}
	statuses := map[string]string{}
	for _, id := range []string{first, second} {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM identity.principal WHERE principal_id = $1::uuid FOR UPDATE`, id).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrIdentityNotFound
			}
			return fmt.Errorf("lock principal %s: %w", id, err)
		}
		statuses[id] = status
	}
	if statuses[sourcePrincipalID] == "ARCHIVED" || statuses[targetPrincipalID] == "ARCHIVED" {
		return ErrMergeNotEligible
	}

	extRows, err := tx.Query(ctx, `
		UPDATE identity.external_identity SET principal_id = $2::uuid
		WHERE principal_id = $1::uuid
		RETURNING issuer, subject, provider_type, status`, sourcePrincipalID, targetPrincipalID)
	if err != nil {
		return fmt.Errorf("transfer external identities: %w", err)
	}
	var transferredExternal []map[string]string
	for extRows.Next() {
		var issuer, subject, providerType, status string
		if err := extRows.Scan(&issuer, &subject, &providerType, &status); err != nil {
			extRows.Close()
			return fmt.Errorf("scan transferred external identity: %w", err)
		}
		transferredExternal = append(transferredExternal, map[string]string{"issuer": issuer, "subject": subject, "provider_type": providerType, "status": status})
	}
	extRowsErr := extRows.Err()
	extRows.Close()
	if extRowsErr != nil {
		return fmt.Errorf("transfer external identities: %w", extRowsErr)
	}

	refRows, err := tx.Query(ctx, `
		UPDATE identity.identity_reference SET principal_id = $2::uuid
		WHERE principal_id = $1::uuid
		RETURNING engine, external_type, external_id`, sourcePrincipalID, targetPrincipalID)
	if err != nil {
		return fmt.Errorf("transfer identity references: %w", err)
	}
	var transferredReferences []map[string]string
	for refRows.Next() {
		var engine, externalType, externalID string
		if err := refRows.Scan(&engine, &externalType, &externalID); err != nil {
			refRows.Close()
			return fmt.Errorf("scan transferred identity reference: %w", err)
		}
		transferredReferences = append(transferredReferences, map[string]string{"engine": engine, "external_type": externalType, "external_id": externalID})
	}
	refRowsErr := refRows.Err()
	refRows.Close()
	if refRowsErr != nil {
		return fmt.Errorf("transfer identity references: %w", refRowsErr)
	}

	if _, err = tx.Exec(ctx, `UPDATE identity.principal SET status = 'ARCHIVED', updated_at = now() WHERE principal_id = $1::uuid`, sourcePrincipalID); err != nil {
		return fmt.Errorf("archive source principal: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"source_principal_id":             sourcePrincipalID,
		"target_principal_id":             targetPrincipalID,
		"transferred_external_identities": transferredExternal,
		"transferred_identity_references": transferredReferences,
	})
	if err != nil {
		return fmt.Errorf("marshal merge audit payload: %w", err)
	}
	for _, target := range []string{"principal:" + sourcePrincipalID, "principal:" + targetPrincipalID} {
		if _, err = tx.Exec(ctx, `
			INSERT INTO audit_events(actor_id, actor_type, client_id, token_id, correlation_id, action, target, result, policy_decision, payload)
			VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, '')::uuid, $6, $7, $8, $9, $10)`,
			actor.ActorID, actor.ActorType, actor.ClientID, actor.TokenID, actor.CorrelationID,
			"identity.principal.merged", target, "success", reason, payload); err != nil {
			return fmt.Errorf("write merge audit record: %w", err)
		}
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
		var bindingMode string
		if err := rows.Scan(
			&b.ID,
			&b.CapabilityKey,
			&b.EngineID,
			&b.EngineInstanceID,
			&bindingMode,
			&b.Priority,
			&b.Status,
			&b.ContractVersion,
			&b.ScopeID,
		); err != nil {
			return nil, err
		}
		b.BindingMode = capabilitydomain.BindingMode(bindingMode)
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
		WHERE c.code=$1`, binding.CapabilityKey, binding.EngineID, binding.EngineInstanceID, binding.ScopeID, string(binding.BindingMode), binding.Priority, binding.Status, binding.ContractVersion)
	return err
}

func (r *PostgresRepository) SaveBinding(ctx context.Context, binding resolver.CapabilityBinding, expectedVersion int64) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	result, err := r.pool.Exec(ctx, `UPDATE capability.capability_binding cb SET binding_mode=UPPER($2), priority=$3, status=UPPER($4), contract_version=$5, version=version+1, updated_at=now() FROM capability.capability c WHERE cb.id=$1::uuid AND cb.capability_id=c.capability_id AND c.code=$6 AND cb.version=$7`, binding.ID, string(binding.BindingMode), binding.Priority, binding.Status, binding.ContractVersion, binding.CapabilityKey, expectedVersion)
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

func (r *PostgresRepository) CreateCapabilityScope(ctx context.Context, scope capabilitydomain.CapabilityScope) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := scope.Validate(); err != nil {
		return fmt.Errorf("validate capability scope: %w", err)
	}
	metadata, err := json.Marshal(scope.Metadata)
	if err != nil {
		return fmt.Errorf("marshal capability scope metadata: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO capability.capability_scope(
			scope_id, tenant_id, legal_entity_id, organisation_id, business_unit_id,
			digital_estate_id, digital_property_id, channel_id, market_id, jurisdiction,
			currency_code, customer_segment_id, catalogue_id, operating_region_id, geographic_region_id,
			deployment_region, environment, isolation_profile_id, include_countries, exclude_countries, metadata
		)
		VALUES (
			$1::uuid, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
			NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''),
			NULLIF($16, ''), NULLIF($17, ''), NULLIF($18, '')::uuid, $19, $20, $21
		)`,
		scope.ScopeID, scope.TenantID, scope.LegalEntityID, scope.OrganisationID, scope.BusinessUnitID,
		scope.DigitalEstateID, scope.DigitalPropertyID, scope.ChannelID, scope.MarketID, scope.Jurisdiction,
		scope.CurrencyCode, scope.CustomerSegmentID, scope.CatalogueID, scope.OperatingRegionID, scope.GeographicRegionID,
		scope.DeploymentRegion, scope.Environment, scope.IsolationProfileID, scope.IncludeCountries, scope.ExcludeCountries, metadata,
	)
	return err
}

const capabilityScopeSelectColumns = `
	scope_id::text, tenant_id, COALESCE(legal_entity_id, ''), COALESCE(organisation_id, ''), COALESCE(business_unit_id, ''),
	COALESCE(digital_estate_id, ''), COALESCE(digital_property_id, ''), COALESCE(channel_id, ''), COALESCE(market_id, ''), COALESCE(jurisdiction, ''),
	COALESCE(currency_code, ''), COALESCE(customer_segment_id, ''), COALESCE(catalogue_id, ''), COALESCE(operating_region_id, ''), COALESCE(geographic_region_id, ''),
	COALESCE(deployment_region, ''), COALESCE(environment, ''), COALESCE(isolation_profile_id::text, ''), include_countries, exclude_countries, metadata,
	created_at, updated_at`

func scanCapabilityScope(row interface {
	Scan(dest ...any) error
}) (capabilitydomain.CapabilityScope, error) {
	var s capabilitydomain.CapabilityScope
	var metadata []byte
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&s.ScopeID, &s.TenantID, &s.LegalEntityID, &s.OrganisationID, &s.BusinessUnitID,
		&s.DigitalEstateID, &s.DigitalPropertyID, &s.ChannelID, &s.MarketID, &s.Jurisdiction,
		&s.CurrencyCode, &s.CustomerSegmentID, &s.CatalogueID, &s.OperatingRegionID, &s.GeographicRegionID,
		&s.DeploymentRegion, &s.Environment, &s.IsolationProfileID, &s.IncludeCountries, &s.ExcludeCountries, &metadata,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return capabilitydomain.CapabilityScope{}, err
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &s.Metadata); err != nil {
			return capabilitydomain.CapabilityScope{}, fmt.Errorf("unmarshal capability scope metadata: %w", err)
		}
	}
	s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	s.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return s, nil
}

func (r *PostgresRepository) GetCapabilityScope(ctx context.Context, scopeID string) (capabilitydomain.CapabilityScope, error) {
	if r == nil || r.pool == nil {
		return capabilitydomain.CapabilityScope{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `SELECT `+capabilityScopeSelectColumns+` FROM capability.capability_scope WHERE scope_id=$1::uuid`, scopeID)
	scope, err := scanCapabilityScope(row)
	if err != nil {
		return capabilitydomain.CapabilityScope{}, fmt.Errorf("get capability scope %s: %w", scopeID, err)
	}
	return scope, nil
}

func (r *PostgresRepository) ListCapabilityScopes(ctx context.Context, tenantID string) ([]capabilitydomain.CapabilityScope, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `SELECT `+capabilityScopeSelectColumns+` FROM capability.capability_scope WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []capabilitydomain.CapabilityScope
	for rows.Next() {
		scope, err := scanCapabilityScope(rows)
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

// CreateGrant resolves capability_id from the caller's CapabilityKey via
// capability.capability.code, mirroring CreateBinding's join pattern.
// Unlike CreateBinding, this checks RowsAffected: an unknown capability_key
// SHALL fail explicitly rather than silently insert nothing.
func (r *PostgresRepository) CreateGrant(ctx context.Context, grant capabilitydomain.CapabilityGrant) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := grant.Validate(); err != nil {
		return fmt.Errorf("validate capability grant: %w", err)
	}
	constraints, err := json.Marshal(grant.Constraints)
	if err != nil {
		return fmt.Errorf("marshal capability grant constraints: %w", err)
	}
	result, err := r.pool.Exec(ctx, `
		INSERT INTO capability.capability_grant(grant_id, tenant_id, capability_id, scope_id, source, source_reference, status, effective_from, effective_to, constraints, granted_by)
		SELECT $1::uuid, $2, c.capability_id, $4::uuid, $5, NULLIF($6, ''), $7, $8, $9, $10, NULLIF($11, '')
		FROM capability.capability c
		WHERE c.code = $3`,
		grant.ID, grant.TenantID, grant.CapabilityKey, grant.ScopeID, string(grant.Source),
		grant.SourceReference, string(grant.Status), grant.EffectiveFrom, grant.EffectiveTo, constraints, grant.GrantedBy,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("capability %q not found", grant.CapabilityKey)
	}
	return nil
}

func (r *PostgresRepository) ListGrants(ctx context.Context, tenantID, capabilityKey string) ([]capabilitydomain.CapabilityGrant, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			cg.grant_id::text, cg.tenant_id, cap.code, cg.scope_id::text,
			cg.source, COALESCE(cg.source_reference, ''), cg.status,
			cg.effective_from, cg.effective_to, cg.constraints,
			COALESCE(cg.granted_by, ''), cg.revoked_at, COALESCE(cg.revoked_by, ''), COALESCE(cg.revocation_reason, ''),
			cg.version
		FROM capability.capability_grant cg
		JOIN capability.capability cap ON cap.capability_id = cg.capability_id
		WHERE cg.tenant_id = $1 AND cap.code = $2
		ORDER BY cg.created_at DESC`, tenantID, capabilityKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []capabilitydomain.CapabilityGrant
	for rows.Next() {
		var g capabilitydomain.CapabilityGrant
		var source, status string
		var constraints []byte
		if err := rows.Scan(
			&g.ID, &g.TenantID, &g.CapabilityKey, &g.ScopeID,
			&source, &g.SourceReference, &status,
			&g.EffectiveFrom, &g.EffectiveTo, &constraints,
			&g.GrantedBy, &g.RevokedAt, &g.RevokedBy, &g.RevocationReason,
			&g.Version,
		); err != nil {
			return nil, err
		}
		g.Source = capabilitydomain.GrantSource(source)
		g.Status = capabilitydomain.GrantStatus(status)
		if len(constraints) > 0 {
			if err := json.Unmarshal(constraints, &g.Constraints); err != nil {
				return nil, fmt.Errorf("unmarshal capability grant constraints: %w", err)
			}
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeGrant never deletes the row -- a revoked grant SHALL remain
// historically queryable (§12).
func (r *PostgresRepository) RevokeGrant(ctx context.Context, grantID, revokedBy, reason string, expectedVersion int64) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	result, err := r.pool.Exec(ctx, `
		UPDATE capability.capability_grant
		SET status='REVOKED', revoked_at=now(), revoked_by=NULLIF($2, ''), revocation_reason=NULLIF($3, ''), version=version+1, updated_at=now()
		WHERE grant_id=$1::uuid AND version=$4`,
		grantID, revokedBy, reason, expectedVersion)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("capability grant %s version conflict or not found", grantID)
	}
	return nil
}

func (r *PostgresRepository) GetCapability(ctx context.Context, capabilityKey string) (capabilitydomain.Capability, error) {
	if r == nil || r.pool == nil {
		return capabilitydomain.Capability{}, errors.New("repository is not initialized")
	}
	var c capabilitydomain.Capability
	var lifecycle, maturity string
	err := r.pool.QueryRow(ctx, `
		SELECT capability_id::text, code, name, COALESCE(description, ''), COALESCE(domain_key, ''), UPPER(status), UPPER(maturity)
		FROM capability.capability WHERE code = $1`, capabilityKey).
		Scan(&c.ID, &c.Key, &c.Name, &c.Description, &c.DomainKey, &lifecycle, &maturity)
	if err != nil {
		return capabilitydomain.Capability{}, fmt.Errorf("get capability %s: %w", capabilityKey, err)
	}
	c.Lifecycle = capabilitydomain.CapabilityLifecycle(lifecycle)
	c.Maturity = capabilitydomain.CapabilityMaturity(maturity)
	return c, nil
}

// CreateCapability expects capability.ID to already be set by the caller
// (via domain.NewUUIDv7()), matching every other Create* method in this
// package.
func (r *PostgresRepository) CreateCapability(ctx context.Context, capability capabilitydomain.Capability) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := capability.Validate(); err != nil {
		return fmt.Errorf("validate capability: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO capability.capability(capability_id, code, name, description, domain_key, status, maturity)
		VALUES ($1::uuid, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)`,
		capability.ID, capability.Key, capability.Name, capability.Description, capability.DomainKey,
		string(capability.Lifecycle), string(capability.Maturity))
	return err
}

func (r *PostgresRepository) ListCapabilityDependencies(ctx context.Context, capabilityKey string) ([]capabilitydomain.CapabilityDependency, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository is not initialized")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cd.id::text, cap.code, dep.code, cd.dependency_type, COALESCE(cd.version_constraint, ''), COALESCE(cd.condition, ''), cd.status
		FROM capability.capability_dependency cd
		JOIN capability.capability cap ON cap.capability_id = cd.capability_id
		JOIN capability.capability dep ON dep.capability_id = cd.depends_on_capability_id
		WHERE cap.code = $1`, capabilityKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []capabilitydomain.CapabilityDependency
	for rows.Next() {
		var d capabilitydomain.CapabilityDependency
		var dependencyType string
		if err := rows.Scan(&d.ID, &d.CapabilityKey, &d.DependsOnCapability, &dependencyType, &d.VersionConstraint, &d.Condition, &d.Status); err != nil {
			return nil, err
		}
		d.DependencyType = capabilitydomain.DependencyType(dependencyType)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateCapabilityDependency enforces ADR-BCP-003 §8's acyclic requirement
// across the whole REQUIRED-only dependency graph: it loads every existing
// REQUIRED edge (not merely the two capabilities being linked) before
// deciding whether the candidate edge would close a cycle.
func (r *PostgresRepository) CreateCapabilityDependency(ctx context.Context, dependency capabilitydomain.CapabilityDependency) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := dependency.Validate(); err != nil {
		return fmt.Errorf("validate capability dependency: %w", err)
	}
	if dependency.DependencyType == capabilitydomain.DependencyTypeRequired {
		rows, err := r.pool.Query(ctx, `
			SELECT cap.code, dep.code
			FROM capability.capability_dependency cd
			JOIN capability.capability cap ON cap.capability_id = cd.capability_id
			JOIN capability.capability dep ON dep.capability_id = cd.depends_on_capability_id
			WHERE cd.dependency_type = 'REQUIRED'`)
		if err != nil {
			return fmt.Errorf("load required dependency graph: %w", err)
		}
		existing := []capabilitydomain.CapabilityDependency{dependency}
		for rows.Next() {
			var key, dependsOn string
			if err := rows.Scan(&key, &dependsOn); err != nil {
				rows.Close()
				return err
			}
			existing = append(existing, capabilitydomain.CapabilityDependency{CapabilityKey: key, DependsOnCapability: dependsOn, DependencyType: capabilitydomain.DependencyTypeRequired})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if capabilitydomain.HasCapabilityDependencyCycle(existing) {
			return fmt.Errorf("adding %s -> %s would create a required-dependency cycle", dependency.CapabilityKey, dependency.DependsOnCapability)
		}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO capability.capability_dependency(capability_id, depends_on_capability_id, dependency_type, version_constraint, condition)
		SELECT cap.capability_id, dep.capability_id, $3, NULLIF($4, ''), NULLIF($5, '')
		FROM capability.capability cap, capability.capability dep
		WHERE cap.code = $1 AND dep.code = $2`,
		dependency.CapabilityKey, dependency.DependsOnCapability, string(dependency.DependencyType), dependency.VersionConstraint, dependency.Condition)
	return err
}

// CreateContext persists a resolved Context (ADR-BCP-004 §70/§73,
// context.resolved_context -- migration 000031). resolved.ID is expected to
// already be a caller-minted UUIDv7 (domain.NewUUIDv7()), matching every
// other first-class resource this repository creates -- there is no
// RETURNING id here.
func (r *PostgresRepository) CreateContext(ctx context.Context, resolved domain.Context) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	if err := resolved.Validate(); err != nil {
		return fmt.Errorf("validate context: %w", err)
	}
	if resolved.ID == "" {
		return errors.New("context id is required")
	}
	provenance, err := json.Marshal(resolved.Provenance)
	if err != nil {
		return fmt.Errorf("marshal context provenance: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO context.resolved_context(
			context_id, principal_id, tenant_id, legal_entity_id, organisation_id, business_unit_id,
			digital_estate_id, digital_property_id, channel_id, market_id, jurisdiction,
			country_code, currency_code, locale, deployment_region, environment, isolation_profile_id,
			correlation_id, resolved_at, expires_at, provenance
		)
		VALUES (
			$1::uuid, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
			NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''),
			NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''), NULLIF($17, ''),
			$18, $19, $20, $21
		)`,
		resolved.ID, resolved.PrincipalID, resolved.TenantID, resolved.LegalEntityID, resolved.OrganisationID, resolved.BusinessUnitID,
		resolved.DigitalEstateID, resolved.DigitalPropertyID, resolved.ChannelID, resolved.MarketID, resolved.Jurisdiction,
		resolved.CountryCode, resolved.CurrencyCode, resolved.Locale, resolved.DeploymentRegion, resolved.Environment, resolved.IsolationProfileID,
		resolved.CorrelationID, resolved.ResolvedAt, resolved.ExpiresAt, provenance,
	)
	return err
}

const resolvedContextSelectColumns = `
	context_id::text, principal_id, tenant_id, COALESCE(legal_entity_id, ''), COALESCE(organisation_id, ''), COALESCE(business_unit_id, ''),
	COALESCE(digital_estate_id, ''), COALESCE(digital_property_id, ''), COALESCE(channel_id, ''), COALESCE(market_id, ''), COALESCE(jurisdiction, ''),
	COALESCE(country_code, ''), COALESCE(currency_code, ''), COALESCE(locale, ''), COALESCE(deployment_region, ''), COALESCE(environment, ''), COALESCE(isolation_profile_id, ''),
	correlation_id, resolved_at, expires_at, provenance`

func scanResolvedContext(row interface {
	Scan(dest ...any) error
}) (domain.Context, error) {
	var c domain.Context
	var provenance []byte
	err := row.Scan(
		&c.ID, &c.PrincipalID, &c.TenantID, &c.LegalEntityID, &c.OrganisationID, &c.BusinessUnitID,
		&c.DigitalEstateID, &c.DigitalPropertyID, &c.ChannelID, &c.MarketID, &c.Jurisdiction,
		&c.CountryCode, &c.CurrencyCode, &c.Locale, &c.DeploymentRegion, &c.Environment, &c.IsolationProfileID,
		&c.CorrelationID, &c.ResolvedAt, &c.ExpiresAt, &provenance,
	)
	if err != nil {
		return domain.Context{}, err
	}
	if len(provenance) > 0 {
		if err := json.Unmarshal(provenance, &c.Provenance); err != nil {
			return domain.Context{}, fmt.Errorf("unmarshal context provenance: %w", err)
		}
	}
	c.ResolvedAt = c.ResolvedAt.UTC()
	if c.ExpiresAt != nil {
		expiresAt := c.ExpiresAt.UTC()
		c.ExpiresAt = &expiresAt
	}
	return c, nil
}

// GetContext returns a resolved Context by id, treating an expired row
// identically to a missing one (ErrContextNotFound in both cases):
// ADR-BCP-004 §72 means the same next step -- re-resolve -- either way, so
// callers have no reason to tell the two apart.
func (r *PostgresRepository) GetContext(ctx context.Context, contextID string) (domain.Context, error) {
	if r == nil || r.pool == nil {
		return domain.Context{}, errors.New("repository is not initialized")
	}
	row := r.pool.QueryRow(ctx, `SELECT `+resolvedContextSelectColumns+`
		FROM context.resolved_context
		WHERE context_id = $1::uuid AND (expires_at IS NULL OR expires_at > now())`, contextID)
	resolved, err := scanResolvedContext(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Context{}, ErrContextNotFound
		}
		return domain.Context{}, fmt.Errorf("get context %s: %w", contextID, err)
	}
	return resolved, nil
}

// DeleteContextsByTenant removes every resolved Context for tenantID
// (ADR-BCP-004 §74, "Context Cache Invalidation") and returns the number of
// rows removed.
func (r *PostgresRepository) DeleteContextsByTenant(ctx context.Context, tenantID string) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("repository is not initialized")
	}
	result, err := r.pool.Exec(ctx, `DELETE FROM context.resolved_context WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return errors.New("repository is not initialized")
	}
	return r.pool.Ping(ctx)
}
