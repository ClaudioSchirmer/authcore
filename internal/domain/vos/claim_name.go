// Hand-written: declared in the spec as `kind: manual`. What puts it beyond a
// regex is the same anti-junk pair this service already shares — a
// distinct-rune floor and a cap on identical runs — plus the reserved-prefix
// pair, which no pattern and no member list states.
//
// It is NOT RoleKey, and that is a decision rather than an oversight. A role
// key is a hyphen slug; a claim name is snake_case, because that is what every
// claim this service already mints looks like (tenant_id, tenant_workspace,
// must_change_password). And RoleKey carries no prefix rule at all.
//
// THE PREFIX IS CALLER-OWNED, AND THAT IS THE WHOLE POINT OF THIS FILE.
// The caller types `x_cost_center`; this type accepts or refuses that string;
// the column stores it verbatim and a token would mint it verbatim. NOTHING
// HERE PREPENDS THE PREFIX AND NOTHING STRIPS IT. The alternative — the server
// prepending a bare `cost_center` — was weighed at the model gate and refused:
// it would make the wire name and the token name differ, so a consumer reading
// `x_cost_center` out of a JWT and searching the catalog for it would find
// nothing. That is the postiche-internal-name shape backlog.md rejects by name.
//
// The consequence, and it is intended: the platform's own nine claims carry no
// prefix, so they cannot be written through this API at all. They enter by
// migration, the same door the platform's `*:*` role enters by — which is what
// frees this entity from the reserved-platform-tenant dependency that Role and
// Group both carry.

package vos

import (
	"regexp"
	"strings"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ClaimNameReservedPrefix is the namespace every tenant-defined claim carries.
//
// Exported so the reserved prefix has exactly ONE spelling in this service: a
// test asserts it, and any future code that has to reason about the namespace
// reads it here rather than repeating the literal. Two runes, chosen against
// `ext_` (two runes dearer) and against a URI namespace (safest and by far the
// most expensive per token — and this issuer's audience is its own mesh, not
// the open internet).
const ClaimNameReservedPrefix = "x_"

const (
	// The bounds cover the WHOLE value, prefix included. Four is the floor
	// because the prefix costs two and the meaningful part needs at least two:
	// `x_hr` is a legitimate claim name, `x_h` is a typo and `x_` is nothing.
	claimNameMinRunes = 4
	// 64 matches the column, sized to the same slug budget the rest of this
	// service uses for machine handles.
	claimNameMaxRunes = 64

	// The anti-junk bounds are applied to the REMAINDER, never to the whole
	// value: the prefix is two runes this type puts there itself, and counting
	// them would inflate every name's distinct-rune score by two and weaken the
	// floor for exactly the values it exists to catch.
	claimNameRemainderMinRunes  = 2
	claimNameMaxIdenticalRun    = 4
	claimNameRemainderMinDistct = 2
)

// claimNameRemainderPattern is lowercase alphanumerics in underscore-separated
// groups, so an underscore can never lead, trail or double inside the
// meaningful part of the name. A leading digit is allowed within a group for
// the same reason the role handle allows it: `x_2fa_enabled` is a claim name
// somebody really wants.
var claimNameRemainderPattern = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// ClaimName is a claim's name exactly as a token carries it: the reserved
// namespace prefix followed by a lowercase snake_case handle, unique within its
// tenant and immutable once set.
type ClaimName string

func (v ClaimName) Value() string { return string(v) }

// IsValid enforces the whole shape in one pass.
//
// NOTHING IS NORMALIZED. `X_Cost_Center`, ` x_cost_center ` and `cost_center`
// are all refused, never quietly repaired — the same stance TenantWorkspace and
// RoleKey take, and for the same reason: the value is immutable, so a caller
// who believes they registered one string and finds another has no second
// chance to correct it.
//
// It raises exactly ONE InvalidClaimNameNotification however many of the shape
// rules failed, because they all say the same thing to a caller; emitting it
// per broken rule would show the identical sentence four times for one bad
// value. Emptiness is the one separate answer, and it is the framework's own
// RequiredFieldNotification — which is exactly why the aggregate declares no
// `required` rule on top of this type.
func (v ClaimName) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotificationNamed(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)
	if length < claimNameMinRunes || length > claimNameMaxRunes || !v.hasReservedPrefix() {
		ctx.AddNotificationNamed(fieldName, InvalidClaimNameNotification{}, s)
		return false
	}

	remainder := s[len(ClaimNameReservedPrefix):]
	wellFormed := runeLen(remainder) >= claimNameRemainderMinRunes &&
		claimNameRemainderPattern.MatchString(remainder) &&
		// The paste error a caller-owned prefix makes possible:
		// `x_x_cost_center` is well-formed snake_case that starts with the
		// prefix, so nothing above catches it. Refused, never trimmed back to
		// one prefix — trimming would be the normalization this type refuses.
		!strings.HasPrefix(remainder, ClaimNameReservedPrefix) &&
		// A name has to carry a letter: `x_123` is an id, not a claim name.
		hasLetter(remainder) &&
		distinctRunes(remainder) >= claimNameRemainderMinDistct &&
		!hasRunOfIdenticalRunes(remainder, claimNameMaxIdenticalRun)

	if !wellFormed {
		ctx.AddNotificationNamed(fieldName, InvalidClaimNameNotification{}, s)
		return false
	}
	return true
}

// hasReservedPrefix reports whether the value carries the namespace every
// tenant-defined claim must declare.
//
// Case-SENSITIVE, deliberately: the whole value is lowercase by the pattern
// below, so an uppercase `X_` is a malformed name rather than a prefix that
// needs folding. Folding it here would be the first normalization step, and
// there are none in this type.
func (v ClaimName) hasReservedPrefix() bool {
	return strings.HasPrefix(string(v), ClaimNameReservedPrefix)
}
