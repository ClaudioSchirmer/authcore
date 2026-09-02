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
	"github.com/ClaudioSchirmer/authcore/internal/infra/dtos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
)

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
func CallerLacksAnyPermissionOfRole(identity *configuration.Identity, row dtos.RoleRow) bool {
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
