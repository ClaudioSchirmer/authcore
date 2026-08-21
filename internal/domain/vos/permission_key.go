package vos

import (
	"regexp"
	"strings"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// PermissionKey is a permission: what resource, and what may be done to it.
//
// It is a COMPOSITE value object — a struct that owns its rule and declares no
// Value(), because there is no single scalar to return. That absence is the
// discriminator the framework uses: it tells the schema to decompose the value
// across resource_name and action_name instead of storing a rendering in one
// column. Declaring Value() here would be a boot failure.
//
// # Why one type and not two fields
//
// Resource and Action only mean something together: "read" alone is not a
// permission and "tenant" alone is not one either. One rule in particular is
// expressible ONLY with both values in hand — a wildcard resource forces a
// wildcard action (isUnmatchable below) — and a rule between two fields has
// nowhere to live on a single-scalar type. That rule is the reason this is a
// composite rather than two loose columns.
//
// # Why the parts are plain strings
//
// A value object earns its keep by giving a rule ONE home and by being
// reusable. Both parts are built from ONE shared definition (isSegment), and
// nothing else in this service carries a resource or an action on its own, so
// that home is this type. Two extra named types would be two copies of one
// helper for no reader. Extracting a part later is mechanical and touches no
// column, no wire name and no data.
//
// # Why it renders itself
//
// How a permission is CHECKED and how a permission is WRITTEN are one concept,
// so they live in one type. String() is the only place in this service that
// knows the separator is a colon — the read side's derivation, the write
// responses and any future token issuer all go through it rather than joining
// two strings of their own.
type PermissionKey struct {
	Resource string `labelKey:"PermissionResourceField"`
	Action   string `labelKey:"PermissionActionField"`
}

// Wildcard is the whole-part wildcard, accepted on either half.
//
// It is legal only as an ENTIRE part — never as a segment inside a path
// ("user:*"), never mixed into a slug ("ten*"). The reason is not stylistic:
// the framework's claim matcher honors exactly three shapes — an exact string,
// "resource:*", and "*:*". A partially wildcarded value fits none of them, so
// it would be a row that matches nothing while reading like a grant.
const Wildcard = "*"

// segmentPattern is one slug: lowercase alphanumerics in groups separated by
// single hyphens, so a hyphen can never lead, trail or double.
var segmentPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxPartRunes bounds each stored part, matching the VARCHAR(64) columns.
const maxPartRunes = 64

// maxRenderedRunes is what one catalog entry costs inside a "permissions"
// claim: 64 + ":" + 64. It is a DERIVED bound, not a threshold anything tests —
// the two part bounds already guarantee it (see IsValid). It is declared so the
// budget is stated where the parts are sized, and so that widening a part is
// visibly a decision about the claim as well.
const maxRenderedRunes = maxPartRunes + 1 + maxPartRunes

// isSegment reports whether s is one well-formed slug: 2 to 64 runes, matching
// segmentPattern, with no run of 4 or more identical runes.
//
// Written once and used by both parts — the resource accepts a colon-joined
// path of these, the action accepts exactly one.
func isSegment(s string) bool {
	if n := runeLen(s); n < 2 || n > maxPartRunes {
		return false
	}
	if !segmentPattern.MatchString(s) {
		return false
	}
	return !hasRunOfIdenticalRunes(s, 4)
}

// isResource reports whether s is a well-formed resource: the wildcard, or a
// colon-joined path of segments bounded at 64 runes in total.
//
// The path is admitted because a resource may legitimately be hierarchical —
// the framework's own documentation gates RequirePermission("users:profile:read").
func isResource(s string) bool {
	if s == Wildcard {
		return true
	}
	if runeLen(s) > maxPartRunes {
		return false
	}
	for segment := range strings.SplitSeq(s, ":") {
		if !isSegment(segment) {
			return false
		}
	}
	return true
}

// isAction reports whether s is a well-formed action: the wildcard, or exactly
// ONE segment.
//
// The single-segment rule is what keeps the rendering unambiguous: the last
// colon segment is always the action, so "user:profile:read" parses back
// exactly one way.
func isAction(s string) bool {
	if s == Wildcard {
		return true
	}
	return isSegment(s)
}

// isUnmatchable reports whether the pair is one the claim matcher could never
// honor: a wildcard resource with a specific action.
//
// This is the invariant that made the concept a composite — it cannot be
// stated about either half alone.
func (v PermissionKey) isUnmatchable() bool {
	return v.Resource == Wildcard && v.Action != Wildcard
}

// String renders the permission as a JWT claim carries it and as
// RequirePermission compares it: resource:action.
//
// It is deliberately NOT named Value(): that name is reserved for a
// single-column value object, and declaring it would tell the framework to
// store this rendering in one column instead of decomposing it. Any other name
// is free, and String() additionally satisfies fmt.Stringer, so "%s" and every
// logger render a permission for free.
//
// This is the ONE place the colon lives. Nothing else in this service joins a
// resource and an action.
func (v PermissionKey) String() string {
	return v.Resource + ":" + v.Action
}

// IsValid is the framework's entry point, found by TYPE with no registration
// and run on every write. It reports every problem it finds rather than
// returning at the first, so a caller who got both halves wrong reads both.
//
// Each part's complaint is emitted under that PART's name — "Resource" or
// "Action" — so the caller learns which half to edit; the framework resolves
// the label from the labelKey tag on the part above, which is why the entity
// declares nothing for them. The one whole-key problem is emitted under the
// field's own name, because neither half is the one at fault.
//
// Nothing here normalizes. "Tenant" and " tenant " are REFUSED, never quietly
// repaired: the value is compared byte-for-byte against a token claim, so
// storing something the caller did not send would mean a permission that reads
// right and matches nothing.
func (v PermissionKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	valid := true

	// Presence first, per part. A raw string part has no value object of its
	// own to answer this, so the composite answers it.
	if v.Resource == "" {
		ctx.AddNotification("Resource", domain.RequiredFieldNotification{})
		valid = false
	} else if !isResource(v.Resource) {
		ctx.AddNotification("Resource", InvalidResourceNameNotification{}, v.Resource)
		valid = false
	}

	if v.Action == "" {
		ctx.AddNotification("Action", domain.RequiredFieldNotification{})
		valid = false
	} else if !isAction(v.Action) {
		ctx.AddNotification("Action", InvalidActionNameNotification{}, v.Action)
		valid = false
	}

	// The rule below is about the PAIR. It is only meaningful once both halves
	// are individually well-formed, so a malformed part reports its own problem
	// and stops here rather than collecting a second, derived one.
	if !valid {
		return false
	}

	if v.isUnmatchable() {
		ctx.AddNotification(fieldName, UnmatchablePermissionKeyNotification{}, v.String())
		valid = false
	}

	// There is no rendered-length check, and none is possible: isResource and
	// isAction each bound their part at maxPartRunes, so String() is at most
	// maxPartRunes + 1 + maxPartRunes — exactly maxRenderedRunes. The cap on
	// what one entry costs inside a "permissions" claim is therefore enforced
	// by construction rather than by a branch that could never be taken.
	return valid
}
