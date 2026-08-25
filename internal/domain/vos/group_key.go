// Hand-written: declared in the spec as `kind: manual`. What puts it beyond a
// regex is the anti-junk pair this service already shares — a distinct-rune
// floor and a cap on identical runs — which no pattern states.
//
// It is NOT RoleKey, and that is a decision rather than an oversight. The RULE
// is the same rule, to the rune; the TYPE is not. A Group.Key field typed
// vos.RoleKey reads as a bug in every file it appears in, and RoleKey's own
// notification would tell a caller their ROLE key is malformed while they were
// creating a group.
//
// The duplication is therefore recorded, not accidental: after this entity the
// two are one rule written twice, differing only in what they raise. A later
// vos.Handle taking the field's own notification would collapse them — that is
// a change to Role, which is /omnicore:evolve-entity's job and needs its own
// approval, so it stays out of scope here.

package vos

import (
	"regexp"

	"github.com/ClaudioSchirmer/omnicore/domain"
)

const (
	// Two runes is the floor because real handles are short: "hr", "ap" and
	// "it" are all legitimate group handles a customer would type.
	groupKeyMinRunes = 2
	// 64 matches the column, which is sized to the same slug budget the role
	// handle and the permission catalog's own parts use.
	groupKeyMaxRunes = 64

	groupKeyMaxIdenticalRun = 4
	// Two, not three: a two-rune key has at most two distinct runes, so a
	// higher floor would refuse "hr" — a value this type exists to accept.
	groupKeyMinDistinct = 2
)

// groupKeyPattern is lowercase alphanumerics in hyphen-separated groups, so a
// hyphen can never lead, trail or double. A leading digit is allowed for the
// same reason the tenant handle allows it: "3m-approvers" is a name somebody
// really has.
var groupKeyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// GroupKey is a group's stable machine handle: the value an API caller, an
// audit line and a directory mapping reference, unique within its tenant and
// immutable once set.
type GroupKey string

func (v GroupKey) Value() string { return string(v) }

// IsValid enforces the handle's shape.
//
// NOTHING IS NORMALIZED. " Engineering " and "ENGINEERING" are refused, never
// quietly repaired — the same stance TenantWorkspace and RoleKey take, and for
// the same reason: the value is immutable, so a caller who believes they
// registered one string and finds another has no second chance to correct it.
//
// It raises exactly one problem, because there is only one way to be wrong
// here: there is no reserved list for a group handle to collide with.
func (v GroupKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool {
	s := string(v)
	if s == "" {
		ctx.AddNotification(fieldName, domain.RequiredFieldNotification{})
		return false
	}

	length := runeLen(s)
	wellFormed := length >= groupKeyMinRunes &&
		length <= groupKeyMaxRunes &&
		groupKeyPattern.MatchString(s) &&
		distinctRunes(s) >= groupKeyMinDistinct &&
		!hasRunOfIdenticalRunes(s, groupKeyMaxIdenticalRun)

	if !wellFormed {
		ctx.AddNotification(fieldName, InvalidGroupKeyNotification{}, s)
		return false
	}

	return true
}
