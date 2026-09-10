-- Gate IAM-3 (docs/governance/gate-iam-3-canonical-identity-scope.md), phase
-- 5 of 6: ADR-0004 §23-30 ("Engine-Native Identities" / "External Reference
-- Model") -- mapping a Principal to engine-native actors (Medusa customer,
-- iDempiere AD_User, Payload user, ...). Matches
-- contracts/identity/v1/external-reference.schema.json field-for-field.
--
-- Deliberately a new, identity-scoped table rather than a reuse of
-- registry.external_reference (migration 000010): that table's FK is
-- hard-wired to registry.canonical_entity, which per the scope doc's
-- Decision 2 represents business entities (products), not actors. ADR-0004
-- §24 asks to reuse the existing model "where appropriate", but forcing
-- identity rows into a table shaped for a different domain concept was
-- judged not appropriate -- the same reasoning migration 000026 already
-- recorded for identity.principal vs. registry.canonical_entity.
--
-- engine is a closed set of known systems (the wire contract's own enum),
-- not a FK into topology.engine/topology.engine_instance the way
-- capability.capability_binding's engine_instance_id is (migration
-- 000012) -- that dynamic, admin-registered engine-instance model is a
-- Mapping/Capability-domain concept the identity contract doesn't use.
-- engine_instance_id here is the contract's own optional free-text
-- elaboration ("which instance of the engine this mapping belongs to"),
-- not a foreign key.
CREATE TABLE IF NOT EXISTS identity.identity_reference (
    identity_reference_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES identity.principal(principal_id) ON DELETE CASCADE,
    engine text NOT NULL CHECK (engine IN ('baobab-trade', 'baobab-erp', 'baobab-cms', 'baobab-pulse')),
    engine_instance_id text,
    external_type text NOT NULL,
    external_id text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'HISTORICAL')),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- ADR-0004 §29 ("Engine mappings SHALL include sufficient scope to
    -- avoid collisions"): a given engine-native actor resolves to at most
    -- one Principal -- stronger than registry.external_reference's
    -- UNIQUE(canonical_entity_id, provider, provider_key), which is scoped
    -- per-canonical-entity and so lets two different canonical entities
    -- both claim the same provider/provider_key.
    UNIQUE (engine, external_type, external_id)
);

CREATE INDEX IF NOT EXISTS identity_reference_principal_idx
    ON identity.identity_reference(principal_id);
