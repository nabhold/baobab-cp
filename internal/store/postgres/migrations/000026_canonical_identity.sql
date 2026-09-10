-- Gate IAM-3 (docs/governance/gate-iam-3-canonical-identity-scope.md), phase 1
-- of 6: ADR-0004 ("Canonical Identity and External Identity Mapping")'s
-- CanonicalIdentity/ExternalIdentity(issuer, subject) layer.
--
-- New identity schema, deliberately separate from registry.canonical_entity
-- (which represents business entities -- products, in the one entity_type
-- used so far -- for the Mapping/resolution domain, not actors). The scope
-- doc's Decision 2 records why: registry.canonical_entity's status column
-- defaults to lowercase 'active' where this migration's tables use this
-- codebase's usual uppercase lifecycle vocabulary, and mixing SKUs and
-- people in one table was judged a stretch of ADR-0004 §24's "reuse where
-- appropriate" rather than a clean fit.
--
-- Table names follow the scope doc's Decision 1 (keep "Principal" as the Go
-- and wire-contract name, not "CanonicalIdentity") rather than the table
-- names floated earlier in Decision 2's options -- identity.principal, not
-- identity.canonical_identity, for the same reason Decision 1 exists: avoid
-- a second name for the same concept at the SQL layer too.
CREATE SCHEMA IF NOT EXISTS identity;

-- Matches contracts/identity/v1/principal.schema.json field-for-field
-- (id, actor_type, status, created_at, updated_at). Deliberately minimal
-- per ADR-0004 §3: "It SHALL NOT contain the complete user profile" --
-- human/workload-specific attributes (display_name/email/verified,
-- client_id/owner/environment) are contracts/identity/v1/human-identity.
-- schema.json and workload-identity.schema.json, out of this phase's scope.
--
-- principal_id has no DEFAULT reliance from the Go layer: BCP-GO-001
-- requires app-generated UUIDv7 (domain.NewUUIDv7()) for first-class
-- Control Plane resources, not PostgreSQL's gen_random_uuid() (UUIDv4) --
-- the DEFAULT here exists only as a safety net for direct SQL, matching
-- every other first-class resource table in this schema (e.g.
-- mapping.mapping_scope, mapping.canonical_mapping), not as the actual
-- ID source.
CREATE TABLE IF NOT EXISTS identity.principal (
    principal_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type text NOT NULL CHECK (actor_type IN ('human', 'workload', 'external')),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DISABLED', 'ARCHIVED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS principal_status_idx
    ON identity.principal(status);

-- Matches contracts/identity/v1/external-identity.schema.json
-- field-for-field (provider_type and last_seen_at are optional there, so
-- nullable here; every other field is that schema's own required list).
-- The UNIQUE(issuer, subject) constraint is ADR-0004 §7/§54's core
-- invariant -- "a given external provider subject SHALL resolve to at
-- most one Canonical Identity" -- and doubles as the index identity
-- resolution's lookup (WHERE issuer = $1 AND subject = $2) needs.
CREATE TABLE IF NOT EXISTS identity.external_identity (
    external_identity_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES identity.principal(principal_id) ON DELETE CASCADE,
    issuer text NOT NULL,
    subject text NOT NULL,
    provider_type text,
    status text NOT NULL CHECK (status IN ('ACTIVE', 'UNLINKED', 'DISABLED', 'REVOKED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz,
    UNIQUE (issuer, subject)
);

CREATE INDEX IF NOT EXISTS external_identity_principal_idx
    ON identity.external_identity(principal_id);
