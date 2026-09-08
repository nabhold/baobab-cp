-- Gate 2: database-enforced invariants for the platform-resolution spine.
-- This is deliberately forward-only. Existing canonical UUIDs and history are
-- preserved; values are normalized before constraints are validated.

UPDATE registry.canonical_entity SET status = lower(status);
ALTER TABLE registry.canonical_entity
    ADD CONSTRAINT canonical_entity_status_ck
    CHECK (status IN ('draft', 'validated', 'active', 'deprecated', 'suspended', 'migrating', 'quarantined', 'retired'));

UPDATE topology.engine_instance SET status = upper(status);
ALTER TABLE topology.engine_instance
    ALTER COLUMN status SET DEFAULT 'ACTIVE',
    ADD COLUMN health_status text NOT NULL DEFAULT 'UNKNOWN',
    ADD COLUMN isolation_profile_id uuid REFERENCES policy.isolation_profile(isolation_profile_id),
    ADD COLUMN residency_region text,
    ADD COLUMN effective_from timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN effective_to timestamptz,
    ADD COLUMN valid_period tstzrange
        GENERATED ALWAYS AS (tstzrange(effective_from, effective_to, '[)')) STORED,
    ADD CONSTRAINT engine_instance_status_ck
        CHECK (status IN ('PROVISIONING', 'ACTIVE', 'DRAINING', 'MAINTENANCE', 'FAILED', 'RETIRED')),
    ADD CONSTRAINT engine_instance_health_status_ck
        CHECK (health_status IN ('UNKNOWN', 'HEALTHY', 'DEGRADED', 'UNHEALTHY')),
    ADD CONSTRAINT engine_instance_period_ck
        CHECK (effective_to IS NULL OR effective_to > effective_from);

UPDATE estate.digital_estate SET status = upper(status);
ALTER TABLE estate.digital_estate
    ALTER COLUMN status SET DEFAULT 'ACTIVE',
    ADD CONSTRAINT digital_estate_status_ck
        CHECK (status IN ('DRAFT', 'ACTIVE', 'SUSPENDED', 'RETIRED'));

ALTER TABLE capability.capability
    ADD COLUMN status text NOT NULL DEFAULT 'ACTIVE',
    ADD CONSTRAINT capability_key_ck
        CHECK (code ~ '^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$'),
    ADD CONSTRAINT capability_status_ck
        CHECK (status IN ('DRAFT', 'ACTIVE', 'DEPRECATED', 'RETIRED'));

UPDATE capability.tenant_capability SET status = upper(status);
ALTER TABLE capability.tenant_capability
    ALTER COLUMN status SET DEFAULT 'ENABLED',
    ADD CONSTRAINT tenant_capability_status_ck
        CHECK (status IN ('REQUESTED', 'ENABLED', 'SUSPENDED', 'REVOKED'));

ALTER TABLE mapping.mapping_scope
    ADD COLUMN legal_entity_id text,
    ADD COLUMN country_code text,
    ADD COLUMN digital_estate_id uuid REFERENCES estate.digital_estate(digital_estate_id),
    ADD COLUMN digital_property_id uuid,
    ADD COLUMN channel_id text,
    ADD COLUMN currency_code text,
    ADD COLUMN locale text,
    ADD COLUMN environment text,
    ADD COLUMN engine_id uuid REFERENCES topology.engine(engine_id),
    ADD COLUMN engine_instance_id uuid REFERENCES topology.engine_instance(engine_instance_id),
    ADD CONSTRAINT mapping_scope_type_ck
        CHECK (scope_type IN ('global', 'tenant', 'legal_entity', 'market', 'digital_estate', 'engine_instance')),
    ADD CONSTRAINT mapping_scope_country_ck
        CHECK (country_code IS NULL OR country_code ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT mapping_scope_currency_ck
        CHECK (currency_code IS NULL OR currency_code ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT mapping_scope_engine_pair_ck
        CHECK (engine_instance_id IS NULL OR engine_id IS NOT NULL);

ALTER TABLE policy.tenant_isolation_profile
    ADD COLUMN valid_period tstzrange
        GENERATED ALWAYS AS (tstzrange(effective_from, effective_to, '[)')) STORED,
    ADD CONSTRAINT tenant_isolation_profile_period_ck
        CHECK (effective_to IS NULL OR effective_to > effective_from),
    ADD CONSTRAINT tenant_isolation_profile_active_excl
        EXCLUDE USING gist (tenant_id WITH =, valid_period WITH &&);

ALTER TABLE market.market_assignment
    ADD COLUMN valid_period tstzrange
        GENERATED ALWAYS AS (tstzrange(effective_from, effective_to, '[)')) STORED,
    ADD CONSTRAINT market_assignment_period_ck
        CHECK (effective_to IS NULL OR effective_to > effective_from),
    ADD CONSTRAINT market_assignment_active_excl
        EXCLUDE USING gist (tenant_id WITH =, market_id WITH =, valid_period WITH &&);

UPDATE mapping.canonical_mapping SET status = lower(status);
ALTER TABLE mapping.canonical_mapping
    ADD CONSTRAINT canonical_mapping_status_ck
        CHECK (status IN ('draft', 'active', 'deprecated', 'suspended', 'quarantined', 'retired')),
    ADD CONSTRAINT canonical_mapping_distinct_entities_ck
        CHECK (source_entity_id <> target_entity_id);

UPDATE capability.capability_binding
SET status = upper(status), binding_mode = upper(binding_mode);
ALTER TABLE capability.capability_binding
    ADD CONSTRAINT capability_binding_status_ck
        CHECK (status IN ('DRAFT', 'ACTIVE', 'SUSPENDED', 'RETIRED')),
    ADD CONSTRAINT capability_binding_priority_ck
        CHECK (priority >= 0),
    ADD CONSTRAINT capability_binding_fallback_ck
        CHECK (fallback_binding_id IS NULL OR fallback_binding_id <> id);

ALTER TABLE registry.external_reference
    ADD CONSTRAINT external_reference_provider_ck
        CHECK (btrim(provider) <> ''),
    ADD CONSTRAINT external_reference_provider_key_ck
        CHECK (btrim(provider_key) <> '');

CREATE INDEX engine_instance_eligibility_idx
    ON topology.engine_instance(engine_id, environment, region, status, health_status)
    WHERE status IN ('ACTIVE', 'DRAINING');

CREATE INDEX mapping_scope_resolution_idx
    ON mapping.mapping_scope(tenant_id, legal_entity_id, market_id, digital_estate_id, engine_instance_id);
