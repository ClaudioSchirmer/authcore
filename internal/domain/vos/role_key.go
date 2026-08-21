package vos

import (
	"regexp"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// RoleKey is a role's stable machine handle — the value an API caller passes,
// an audit line records, and an operator reads in a grant listing.
//
// # Why it is not a reuse of TenantWorkspace
//
// The two share a shape and nothing else. TenantWorkspace carries the
// platform's reserved-route list and DeriveTenantID, and both are about the
// PLATFORM: the reserved list is this service's own routes, and the derivation
// mints a public key. A role handle is scoped to one tenant, reaches no
// hostname and feeds no derivation, so reusing that type would drag two rules
// onto a role that have no meaning there — "billing" would be refused as
// reserved when it is exactly what a customer would call the role.
//
// # Why it is not a generated raw value object
//
// The shape is a regex, but the substance checks are not: "at least this many
// distinct runes" and "no run of four identical runes" are the shared anti-junk
// predicates, and no pattern states them. They are what separates a handle from
// a held key.
//
// # Why the bounds are 2 and 64
//
// Two at the bottom, because real handles are short — "hr", "qa", "it" are
// ordinary role names and a three-rune floor would refuse them. Sixty-four at
// the top, matching the permission key's part bound, so a role handle can never
// be the reason a rendered claim outgrows what the catalog already admits.
//
// # Why it is immutable, enforced elsewhere
//
// Nothing here says so — immutability compares against the previous value, so
// it is a rule on the entity (role-key-immutable) and not a property of the
// value. What this type guarantees is only that the value is well formed.
type RoleKey string

// roleKeyPattern is one slug: lowercase alphanumerics in groups separated by
// single hyphens, so a hyphen can never lead, trail or double.
//
// Deliberately the same shape as a permission key's segment and a tenant's
// workspace, and deliberately a separate variable: three handles that happen to
// agree today are not one rule, and sharing the variable would make a change to
// any of them silently move the other two.
var roleKeyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Value is the underlying primitive the persister binds and the mappers read.
func (v RoleKey) Value() string { return string(v) }

// IsValid is the framework's entry point. It is found by TYPE, with no
// registration, and runs on every write.
//
// It reports every problem it finds through the context instead of returning at
// the first, so one call tells the caller everything that is wrong with the
// value.
//
// Nothing here normalizes. " Billing " and "BILLING" are REFUSED, never quietly
// repaired: the handle is immutable once written, so storing something the
// caller did not send would leave them believing they own a key that no log
// line agrees with, and no second chance to correct it.
func (v RoleKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)

	// Presence is answered here, and NOT again in BuildRules: a rule declaring
	// this field required would make the caller read the same complaint twice
	// for one empty value.
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	valid := true
	invalid := func() {
		if valid {
			ctx.AddNotification(fieldName, InvalidRoleKeyNotification{}, s)
			valid = false
		}
	}

	if n := runeLen(s); n < 2 || n > 64 {
		invalid()
	}
	if !roleKeyPattern.MatchString(s) {
		invalid()
	}
	// At least 3 distinct runes, or as many as the value is long when it is
	// shorter than that. A flat "at least 3" would be unsatisfiable at the
	// two-rune floor this type deliberately admits, and would reject hr and qa.
	if distinctRunes(s) < min(3, runeLen(s)) {
		invalid()
	}
	if hasRunOfIdenticalRunes(s, 4) {
		invalid()
	}

	return valid
}
