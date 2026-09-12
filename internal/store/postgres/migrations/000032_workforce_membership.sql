-- Gate IAM-5 phase 3 (docs/governance/gate-iam-5-workforce-sso-scope.md,
-- baobab-iam), ADR-0009 §27 ("Workforce Membership"):
--
--   CanonicalIdentity -> WorkforceMembership -> {LegalEntity, Tenant, status}
--
-- Lives in the identity schema (000026_canonical_identity.sql), not
-- alongside tenants/legal_entities in public -- this is fundamentally an
-- identity-domain relationship (which workforce Principal belongs to which
-- tenant, with what role), the same reasoning identity.external_identity
-- already follows for its own principal_id foreign key.
--
-- roles is text[], matching the existing include_countries/exclude_countries
-- convention (000029_capability_scope_and_grant.sql) for a small,
-- application-validated string set -- not a separate join table, since
-- ADR-0009 §102-104 keeps the realm-role namespace deliberately small and
-- role membership here is never queried by role alone (always scoped to a
-- known principal_id/tenant_id first).
CREATE TABLE IF NOT EXISTS identity.workforce_membership (
    membership_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES identity.principal(principal_id) ON DELETE CASCADE,
    tenant_id varchar(63) NOT NULL REFERENCES tenants(tenant_id),
    legal_entity_id varchar(63),
    roles text[] NOT NULL,
    status text NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DISABLED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- One membership row per (principal, tenant): a mover (ADR-0009 §31-32)
    -- changes this row's roles/status rather than accumulating a second
    -- row for the same tenant, which is exactly the privilege-accumulation
    -- failure mode §32 warns against.
    UNIQUE (principal_id, tenant_id)
);

-- Role-aware authorization's actual lookup shape (api/router.go,
-- GetWorkforceMembership): "does this principal have an ACTIVE membership
-- in this tenant".
CREATE INDEX IF NOT EXISTS workforce_membership_principal_idx
    ON identity.workforce_membership(principal_id);

CREATE INDEX IF NOT EXISTS workforce_membership_tenant_idx
    ON identity.workforce_membership(tenant_id);
