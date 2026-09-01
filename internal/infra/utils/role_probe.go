// Hand-written, and deliberately NOT named for an entity.
//
// This file holds what "one role, resolved" MEANS, and it lives on its own
// because two aggregates ask it: Group, when a role is attached to it, and User,
// when a role is granted directly or reached through a group. It started inside
// group_service_manual.go and moved here the day the second caller appeared —
// the type is about a ROLE, so hanging User off a file named for Group would
// have meant that removing Group breaks User for a reason that has nothing to do
// with groups.
//
// What is NOT here is the resolution: the read, the memo and the repositories
// stay on each service, because those are per-service concerns. What is shared
// is the SHAPE and the one judgement that must never differ between callers —
// what an unresolvable role means.

package utils

import (
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
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

// CallerLacksAnyPermissionOfRole is the shared body of every escalation fact in
// this package — User's two (direct and through a group) and Client's.
//
// It lives beside the row it judges for the reason this whole file exists: one
// definition of "holds every key", one wildcard guard, one treatment of an
// unresolvable role. Copies would be one rule that can disagree with itself
// about the case that matters most, and it is a SECURITY rule.
//
// It judges EVERY key the role confers, including one whose catalog row has
// since been retired. That is the fail-closed direction — a role whose bundle
// contains a retired `tenant:export` is refused to a caller who does not hold
// `tenant:export` — and since the grants no longer carry the catalog row's
// archive stamp it is also the only reading expressible without a second query.
// Strictly more restrictive, never more permissive.
func CallerLacksAnyPermissionOfRole(identity *configuration.Identity, row RoleRow) bool {
	if row.GrantsWildcard() {
		// Never handed to HasPermission — it panics on a wildcard, and this
		// also covers the unresolvable role. The wildcard rule refuses this
		// write on its own; this is the second lock.
		return true
	}
	for _, key := range row.Keys {
		if !identity.HasPermission(key.String()) {
			return true
		}
	}
	return false
}
