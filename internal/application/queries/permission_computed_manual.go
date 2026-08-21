// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The derivations for 1 computed read field(s).
//
// entity:     Permission
// spec:       specs/omnicore-gen/permission.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-20
//
// every computed read field renders absent until its derivation is written
// — quietly, because an empty derivation is indistinguishable from one
// that had nothing to say
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package queries

import (
	"github.com/ClaudioSchirmer/omnicore/application/configuration"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
)

// The derivations behind this entity's computed read fields.
//
// Each runs ONCE per document, below the web boundary, so REST, GraphQL and
// the CSV/XLSX export all render the same value — that is the whole reason
// read-side computation lives at this seat instead of in a Response.
//
// Until a body is written the field renders ABSENT, quietly. The framework
// cannot detect that: as far as it is concerned the derivation ran and
// produced nothing.

// ComputePermission derives Permission from Resource, Action.
//
// The permission as a JWT claim carries it and RequirePermission compares it
// — resource:action, rendered from the two stored columns.
//
// A source declared NULLABLE arrives as a pointer, and nil is a value the
// derivation has to decide about; the rest arrive as values, and the caller
// has already established they were fetched.
//
// An error here fails the whole read, so return one only when the derivation
// genuinely cannot produce a value — a missing source is absence, not a
// failure.
func ComputePermission(ctx *configuration.AppContext, resource string, action string) (string, error) {
	// Rebuild the domain type from the two stored columns and ask IT, rather
	// than joining the strings here. The colon lives in exactly one place in
	// this service — vos.PermissionKey.String() — so the rendering the read
	// returns is the same one the write side and any future token issuer
	// produce, and there is no second definition to drift.
	//
	// No validation happens on this path, and none should: reconstruction is a
	// read concern and validity was settled at the write boundary. A row that
	// predates a tightened rule still renders, which is what an audit of an old
	// grant needs.
	return vos.PermissionKey{Resource: resource, Action: action}.String(), nil
}
