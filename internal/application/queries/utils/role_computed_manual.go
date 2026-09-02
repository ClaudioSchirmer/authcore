// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The derivations for 1 computed read field(s).
//
// entity:     Role
// spec:       specs/omnicore-gen/role.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-09-01
//
// every computed read field renders absent until its derivation is written
// — quietly, because an empty derivation is indistinguishable from one
// that had nothing to say
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package utils

import (
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
)

// The derivations behind this entity's computed read fields.
//
// Each runs below the web boundary, so every surface renders the SAME value
// — that is the whole reason read-side computation lives at this seat
// instead of in a Response. A root derivation runs once per document and heads
// a column of the CSV/XLSX export; a collection's runs once per ENTRY, is
// handed that entry, and reaches REST and GraphQL only — a tabular row is
// flat, so no field of a collection is in one, derived or stored.
//
// Until a body is written the field renders ABSENT, quietly. The framework
// cannot detect that: as far as it is concerned the derivation ran and
// produced nothing.

// ComputeRoleRolePermissionPermission derives Permission from Resource,
// Action.
//
// The permission as a token carries it and a route compares it:
// resource:action.
//
// It runs ONCE PER ENTRY of Permissions, and its sources are that entry's own
// fields — the record around it is not in scope, and the framework would not
// fetch it here if it were.
//
// A source declared NULLABLE arrives as a pointer, and nil is a value the
// derivation has to decide about; the rest arrive as values, and the caller
// has already established they were fetched.
//
// An error here fails the whole read, so return one only when the derivation
// genuinely cannot produce a value — a missing source is absence, not a
// failure.
func ComputeRoleRolePermissionPermission(ctx *configuration.AppContext, resource string, action string) (string, error) {
	// Rebuild the value object and ASK it, rather than joining two strings
	// here. The separator has exactly one home in this service, and this is a
	// consumer of the format, not a second definition of it — the permission
	// catalog's own endpoint renders the pair through this same method.
	//
	// It cannot fail: both sources are NOT NULL columns of the catalog reached
	// across an inner join, and the rendering is a pure concatenation, so the
	// error return is the generic shape every derivation is given and not a
	// branch this one can take. The pair was validated on Permission's write
	// side; a value that reached the store passed PermissionKey.IsValid.
	return vos.PermissionKey{Resource: resource, Action: action}.String(), nil
}
