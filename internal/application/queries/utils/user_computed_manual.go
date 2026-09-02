// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The derivations for 1 computed read field(s).
//
// entity:     User
// spec:       specs/omnicore-gen/user.omnicore.yaml
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

// ComputeUserFullName derives FullName from GivenName, FamilyName.
//
// The person's given and family names on one line, for listings.
//
// A source declared NULLABLE arrives as a pointer, and nil is a value the
// derivation has to decide about; the rest arrive as values, and the caller
// has already established they were fetched.
//
// An error here fails the whole read, so return one only when the derivation
// genuinely cannot produce a value — a missing source is absence, not a
// failure.
func ComputeUserFullName(ctx *configuration.AppContext, givenName string, familyName string) (string, error) {
	// DELEGATED, not reimplemented. Joining a given name to a family name is a
	// fact about names rather than about a read model, so the definition lives
	// on the value object that owns both halves — vos.PersonName.FullName().
	//
	// That matters for one concrete reason: the day a locale needs the family
	// name first (ja-JP is the ordinary example), it is one method that
	// changes, and REST, GraphQL, the export and every write response move
	// together. A `givenName + " " + familyName` written here would be a second
	// definition, free to disagree with the one the domain uses.
	//
	// It also handles the halves the framework can hand us: a projection that
	// selected only one of the two sources arrives with the other empty, and
	// FullName() answers with the half it has rather than with a stray space.
	return vos.PersonName{Given: givenName, Family: familyName}.FullName(), nil
}
