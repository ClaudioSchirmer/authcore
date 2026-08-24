// Hand-written: declared in the spec as a composite with `written: manual`.
// The SHAPE stays declared there (the schema decomposes the two parts into
// columns, the mappers fold them, the migration sizes them, the catalogs
// translate them); this file is the rule.
//
// Two things put it beyond the spec language. One is the pair-level invariant
// — a `*` resource forces a `*` action — which no per-part check can see. The
// other is that both parts are built from ONE shared segment rule that reuses
// this package's anti-junk predicates, which no regex states on its own.
//
// It is also the single home of the SEPARATOR. Nothing else in this service
// concatenates a resource and an action with a colon: every consumer of the
// format calls String() — the read side's derivation, the write responses, and
// the token issuer when it arrives.

package vos

import (
	"regexp"
	"strings"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	// A segment is the unit both parts are built from: one lowercase slug.
	permissionSegmentMinRunes = 2
	permissionSegmentMaxRunes = 64

	// The ceiling on each PART, which for the single-segment action is the
	// segment bound and for the colon-joined resource path is the whole thing.
	permissionPartMaxRunes = 64

	permissionMaxIdenticalRun = 4

	// PermissionWildcard is accepted as an ENTIRE part and nowhere else —
	// never as a segment inside a path (`user:*`), never mixed into a slug
	// (`ten*`). The claim matcher honours exactly three shapes: an exact
	// string, `resource:*`, and `*:*`. A partially wildcarded value fits none
	// of them, so it would be a row that matches nothing while reading like a
	// grant. Refusing it here makes that unrepresentable.
	PermissionWildcard = "*"

	// permissionKeySeparator is the one place this service knows what joins a
	// resource to an action.
	permissionKeySeparator = ":"

	// maxRenderedRunes is what the two 64-rune part bounds are FOR: the budget
	// one entry costs inside a `permissions` claim. It is DERIVED, never
	// checked — the part rules run first and cap each half independently, so
	// the rendered form cannot exceed it and a check would be a branch nothing
	// can take. It is declared so that widening a part is visibly a decision
	// about the token as well as about the column.
	maxRenderedRunes = permissionPartMaxRunes*2 + len(permissionKeySeparator)
)

// permissionSegmentPattern is the same relaxed slug the tenant handle uses:
// lowercase alphanumerics in hyphen-separated groups, hyphens never leading,
// trailing or doubled. A segment MAY lead with a digit — a resource named
// after a product line ("3d-assets") is ordinary.
var permissionSegmentPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// PermissionKey is a resource together with what may be done to it — the pair
// a JWT claim carries and a route compares against, byte for byte.
//
// It declares NO Value(): that absence is what tells the framework the value
// spans several columns and has to be decomposed rather than stored as one
// rendering. The canonical rendering lives on String() instead, which is the
// name the framework's docs name for it and which satisfies fmt.Stringer, so
// %s and every logger render the pair for free.
type PermissionKey struct {
	Resource string `labelKey:"PermissionResourceField"`
	Action   string `labelKey:"PermissionActionField"`
}

// String renders the permission the way a token carries it and a route
// compares it: resource:action.
//
// Because the action is always exactly ONE segment, the LAST colon segment of
// the rendering is always the action — so `user:profile:read` parses back
// exactly one way.
func (v PermissionKey) String() string {
	return v.Resource + permissionKeySeparator + v.Action
}

// IsValid is the framework's entry point, found by TYPE with no registration.
//
// fieldName is deliberately unused: a composite reports on its PARTS, not on
// the field carrying it, so every notification below is emitted under
// "Resource" or "Action". That is what lets a caller read WHICH half is wrong,
// and it resolves each part's labelKey from the tag inside this struct without
// the entity declaring anything.
//
// Both parts are checked before the pair is, and neither short-circuits the
// other: a call carrying two malformed halves gets told about both in one
// answer rather than one per round-trip.
func (v PermissionKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	resourceValid := v.validateResource(ctx)
	actionValid := v.validateAction(ctx)
	if !resourceValid || !actionValid {
		return false
	}

	// The pair-level rule, and the reason this concept had to be a composite:
	// it is only expressible with both values in hand. `*:read` is not one of
	// the three shapes the claim matcher honours, so a caller granted it would
	// match no route ever while the catalog row reads like a sweeping grant.
	// It is reported against the action, which is the half that has to change.
	if v.Resource == PermissionWildcard && v.Action != PermissionWildcard {
		ctx.AddNotification("Action", UnmatchablePermissionKeyNotification{}, v.String())
		return false
	}
	return true
}

// validateResource accepts the wildcard on its own, or a colon-joined path of
// segments — one (`tenant`) or several (`user:profile`). The hierarchy is
// deliberate: the framework's own example gates
// RequirePermission("users:profile:read"), so a resource may legitimately be a
// path.
func (v PermissionKey) validateResource(ctx *domain.NotificationContext) bool {
	if v.Resource == "" {
		ctx.AddNotification("Resource", domain.RequiredFieldNotification{})
		return false
	}
	if v.Resource == PermissionWildcard {
		return true
	}
	if runeLen(v.Resource) > permissionPartMaxRunes {
		ctx.AddNotification("Resource", InvalidResourceNameNotification{}, v.Resource)
		return false
	}
	// Split rather than a single pattern: an empty segment (a leading,
	// trailing or doubled colon) then fails the segment rule by itself.
	for _, segment := range strings.Split(v.Resource, permissionKeySeparator) {
		if !isPermissionSegment(segment) {
			ctx.AddNotification("Resource", InvalidResourceNameNotification{}, v.Resource)
			return false
		}
	}
	return true
}

// validateAction accepts the wildcard on its own, or EXACTLY one segment. The
// no-colon rule is what keeps the rendering unambiguous — see String().
func (v PermissionKey) validateAction(ctx *domain.NotificationContext) bool {
	if v.Action == "" {
		ctx.AddNotification("Action", domain.RequiredFieldNotification{})
		return false
	}
	if v.Action == PermissionWildcard {
		return true
	}
	if !isPermissionSegment(v.Action) {
		ctx.AddNotification("Action", InvalidActionNameNotification{}, v.Action)
		return false
	}
	return true
}

// isPermissionSegment reports whether s is one lowercase slug: 2 to 64 runes,
// hyphen-separated alphanumeric groups, and no run of four or more identical
// runes.
//
// No normalization anywhere: `Tenant` and ` tenant ` are refused, never
// repaired. That doctrine matters more here than on any other value of this
// service, because the rendered pair is compared byte-for-byte against a token
// claim — a quietly repaired value would authorize nothing and explain nothing.
func isPermissionSegment(s string) bool {
	length := runeLen(s)
	return length >= permissionSegmentMinRunes &&
		length <= permissionSegmentMaxRunes &&
		permissionSegmentPattern.MatchString(s) &&
		!hasRunOfIdenticalRunes(s, permissionMaxIdenticalRun)
}
