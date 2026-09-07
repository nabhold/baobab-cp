package domain

// Known first-class CanonicalEntity.EntityType values. EntityType itself
// remains a plain string (see canonical.go) — the control plane is
// intentionally data-driven and does not enforce a closed enum here.
// These constants exist so callers spell known kinds consistently rather
// than repeating string literals, and so a new kind's canonical vocabulary
// is documented in one place.
const (
	// EntityTypeProduct is the canonical entity kind for a product, e.g.
	// canonical_key "product:green-coffee:ethiopia-guji".
	EntityTypeProduct = "PRODUCT"

	// EntityTypeSupplierOrganisation is the canonical entity kind for a
	// prospective or approved supplier organisation, e.g. canonical_key
	// "supplier:<estate>:<estate-local-application-id>" (see ADR-0006).
	// It is registered here as a name only: this package does not create,
	// resolve, or map any SUPPLIER_ORGANISATION entity, and no other
	// control-plane code branches on this constant. Registration happens
	// exclusively through the existing entity-type-agnostic
	// CanonicalEntityService.Create API once a hosting estate is ready to
	// call it.
	EntityTypeSupplierOrganisation = "SUPPLIER_ORGANISATION"
)
