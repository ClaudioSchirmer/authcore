// Hand-written, and not a hook: no generator declares this file.
//
// ONE ROLE, RESOLVED — the shape Group, User and Client all read when they ask what
// a role confers. It is a shared STRUCT, so it lives here rather than beside the
// functions that fill it.
//
// GrantsWildcard travels WITH it and not to utils/, because Go puts a method in its
// receiver's package: separating them is not a layout choice, it is impossible.

package dtos

import (
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// RoleRow is everything the three per-entry facts need to know about one role,
// resolved in a single read.
//
// found is false for a role no ACTIVE row carries — which deliberately collapses
// "no such role" and "a role this tenant retired" into one state. The two are
// indistinguishable to every caller of this type, and they have to be: the
// notification these facts feed answers all three of its questions with one
// message, because a distinct reply would be an existence oracle.
//
// keys are the permissions the role confers, already rendered from the grants'
// joined resource and action. ACTIVE grants only — the loader's default child
// scope — because a revoked grant confers nothing and must not be judged.
type RoleRow struct {
	Found    bool
	TenantID domain.ID
	Keys     []vos.PermissionKey
}

// grantsWildcard reports whether the role confers any permission with a wildcard
// in either part.
//
// A role that resolves to nothing answers TRUE. That is deliberate and it is the
// fail-closed direction: an unresolvable attachment must never reach the
// escalation probe, which would hand its key to Identity.HasPermission — and
// that panics on a wildcard and on an empty string alike.
func (r RoleRow) GrantsWildcard() bool {
	if !r.Found {
		return true
	}
	for _, key := range r.Keys {
		if key.Resource == vos.PermissionWildcard || key.Action == vos.PermissionWildcard {
			return true
		}
	}
	return false
}
