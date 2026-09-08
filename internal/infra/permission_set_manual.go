// Hand-written, and not a hook: no generator declares this file.
//
// THE ONE PLACE THIS SERVICE DECIDES WHAT A `permissions` CLAIM CONTAINS. Both
// sign-in readers walk a different graph — a user reaches roles directly and
// through groups, a client reaches them one way — but what they do with the rows
// those walks return is identical, and it has to stay identical: a permission
// must appear in a minted token EXACTLY ONCE, whether the principal is a user or
// a client, and whichever path reached it.
//
// IT USED TO BE WRITTEN TWICE. authentication_reader.go and
// client_authentication_reader.go each carried their own `seen` map, their own
// three gates and their own sort. Nothing made them fail together, so nothing
// would have caught the day one of them changed and the other did not. This file
// is that guarantee, held in one implementation.
//
// NOTHING UPSTREAM CAN REINTRODUCE A REPEAT. RenderPermissions renders exactly
// what this produced, and BuildClaims hands that straight to the Issuer — which,
// per the framework's own contract, never interprets the claim. What leaves here
// is what ships.
//
// A DUPLICATE WAS NEVER AN AUTHORIZATION DEFECT, and saying so is not an argument
// against this file. A consumer parses the claim into a set of its own and caches
// it on the Identity, so a repeated entry would have cost header bytes and
// readability rather than a wrong allow. The guarantee is about what this service
// EMITS, and it is worth stating exactly once.

package infra

import (
	"sort"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
)

// permissionSet collects what a grant walk reached, keeping the first sighting of
// each pair and discarding every later one.
//
// The zero value is ready to use: add allocates on its first call, so a walk that
// reaches nothing pays nothing.
type permissionSet struct {
	seen map[string]struct{}
	keys []vos.PermissionKey
}

// add offers one row of a grant walk, and applies the three gates that decide
// whether it belongs in a token at all.
//
// THE GATES ARE HERE AND NOT IN THE QUERY, deliberately, and the readers explain
// why at length: a role arrives once per grant it confers, so a WHERE dropping a
// revoked grant would drop the ROLE with it — and a role whose every permission
// was revoked is still a role the principal holds. The columns come back; the
// decision is made per row, here.
//
//	res / act nil    the role confers nothing — a LEFT join's empty half
//	grantArch        the role's grant of this permission was revoked
//	permArch         the catalog entry itself was retired
//
// THE DEDUP KEY IS THE RENDERED PAIR, which is also what the token carries, so
// two rows collapse here exactly when they would have been indistinguishable to
// a consumer.
func (s *permissionSet) add(res, act *string, grantArch, permArch *time.Time) {
	switch {
	case res == nil || act == nil:
		return
	case grantArch != nil:
		return
	case permArch != nil:
		return
	}

	key := *res + ":" + *act
	if _, dup := s.seen[key]; dup {
		return
	}
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	s.seen[key] = struct{}{}
	s.keys = append(s.keys, vos.PermissionKey{Resource: *res, Action: *act})
}

// sorted answers what was collected, in a stable order.
//
// The order is what makes two tokens minted from the same grants byte-identical
// in this claim — which is what makes a diff between two tokens readable when
// somebody is working out why a permission disappeared.
//
// AN EMPTY WALK ANSWERS NIL rather than an empty slice, which is what both
// readers answered before this file existed and what their tests pin. It reaches
// no wire in that shape: RenderPermissions builds its own non-nil slice, so the
// claim is always PRESENT and empty — which is how a consumer tells "this
// principal holds nothing" from "this token predates permissions".
func (s *permissionSet) sorted() []vos.PermissionKey {
	sort.Slice(s.keys, func(i, j int) bool {
		a, b := s.keys[i], s.keys[j]
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		return a.Action < b.Action
	})
	return s.keys
}
