// Hand-written: declared in the spec as `kind: manual`. What puts it beyond a
// regex is the anti-junk pair this service already shares — a distinct-rune
// floor and a cap on identical runs — which no pattern states.
//
// It is NOT TenantWorkspace, and that is a decision rather than an oversight.
// The handle carries a rule that belongs to the platform's own routing — the
// reserved-handle list — which a role has no business inheriting. The shape is
// similar; the rules are not the same rules.

package vos

import (
	"regexp"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	// Two runes is the floor because real keys are short: "hr", "ap", "it" are
	// all legitimate role handles a customer would type.
	roleKeyMinRunes = 2
	// 64 matches the column, which is sized to the same slug budget the
	// permission catalog's own parts use.
	roleKeyMaxRunes = 64

	roleKeyMaxIdenticalRun = 4
	// Two, not three: a two-rune key has at most two distinct runes, so a
	// higher floor would refuse "hr" — a value this type exists to accept.
	roleKeyMinDistinct = 2
)

// roleKeyPattern is lowercase alphanumerics in hyphen-separated groups, so a
// hyphen can never lead, trail or double. A leading digit is allowed for the
// same reason the tenant handle allows it: "3m-approvers" is a name somebody
// really has.
var roleKeyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// RoleKey is a role's stable machine handle: the value an API caller and an
// audit line reference, unique within its tenant and immutable once set.
type RoleKey string

func (v RoleKey) Value() string { return string(v) }

// IsValid enforces the handle's shape.
//
// NOTHING IS NORMALIZED. " Billing-Manager " and "BILLING-MANAGER" are refused,
// never quietly repaired — the same stance TenantWorkspace takes, and for the
// same reason: the value is immutable, so a caller who believes they registered
// one string and finds another has no second chance to correct it.
//
// It raises exactly one problem, unlike TenantWorkspace, because there is only
// one way to be wrong here: there is no reserved list to collide with.
func (v RoleKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)
	wellFormed := length >= roleKeyMinRunes &&
		length <= roleKeyMaxRunes &&
		roleKeyPattern.MatchString(s) &&
		distinctRunes(s) >= roleKeyMinDistinct &&
		!hasRunOfIdenticalRunes(s, roleKeyMaxIdenticalRun)

	if !wellFormed {
		ctx.AddNotification(fieldName, InvalidRoleKeyNotification{}, s)
		return false
	}

	return true
}
