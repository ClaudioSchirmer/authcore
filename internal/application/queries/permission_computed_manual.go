// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The derivations for 1 computed read field(s).
//
// entity:     Permission
// spec:       specs/omnicore-gen/permission.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-24
//
// every computed read field renders absent until its derivation is written
// — quietly, because an empty derivation is indistinguishable from one
// that had nothing to say
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package queries

import (
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
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
// The permission as a token carries it and a route compares it:
// resource:action.
//
// A source declared NULLABLE arrives as a pointer, and nil is a value the
// derivation has to decide about; the rest arrive as values, and the caller
// has already established they were fetched.
//
// An error here fails the whole read, so return one only when the derivation
// genuinely cannot produce a value — a missing source is absence, not a
// failure.
func ComputePermission(ctx *configuration.AppContext, resource string, action string) (string, error) {
	// Rebuild the value object and ASK it, rather than joining two strings
	// here. The separator has exactly one home in this service, and this is a
	// consumer of the format, not a second definition of it — the write
	// responses and the future token issuer render the pair through the same
	// method.
	//
	// It cannot fail: both sources are non-nullable columns and the rendering
	// is a pure concatenation, so the error return is the generic shape every
	// derivation is given and not a branch this one can take. Validation
	// already happened on the write side; a value that reached the store
	// passed PermissionKey.IsValid.
	return vos.PermissionKey{Resource: resource, Action: action}.String(), nil
}
