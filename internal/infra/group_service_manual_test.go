// Hand-written: the parts of the Group service that do NOT need a live engine.
//
// `task_tests.md` records that the repository, the service implementation and
// the routes need a relational engine and that their coverage is reported
// honestly rather than padded. That deviation is about the DATABASE probes.
// The pure predicates and the identity paths below reach no store at all, so
// leaving them uncovered would be hiding behind the deviation rather than
// stating it — and three of them are security decisions.

package infra

import (
	"github.com/ClaudioSchirmer/authcore/internal/infra/dtos"
	"testing"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

const probedRoleID = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f61"

func concreteKeys(pairs ...[2]string) []vos.PermissionKey {
	out := make([]vos.PermissionKey, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, vos.PermissionKey{Resource: p[0], Action: p[1]})
	}
	return out
}

// The fail-closed direction, and the reason it is not an oversight: a role id
// that resolves to nothing must answer TRUE, so an unresolvable attachment
// never reaches the escalation probe — which would hand its key to
// Identity.HasPermission, and that panics on a wildcard and on an empty string
// alike.
func TestRoleRowGrantsWildcardFailsClosedOnAnUnknownID(t *testing.T) {
	cases := []struct {
		name string
		row  dtos.RoleRow
		want bool
	}{
		{
			"a bundle of concrete permissions is not a wildcard",
			dtos.RoleRow{Found: true, Keys: concreteKeys([2]string{"tenant", "read"}, [2]string{"role", "insert"})},
			false,
		},
		{
			"one wildcard RESOURCE anywhere in the bundle is",
			dtos.RoleRow{Found: true, Keys: concreteKeys([2]string{"tenant", "read"}, [2]string{vos.PermissionWildcard, vos.PermissionWildcard})},
			true,
		},
		{
			"one wildcard ACTION anywhere in the bundle is",
			dtos.RoleRow{Found: true, Keys: concreteKeys([2]string{"tenant", "read"}, [2]string{"tenant", vos.PermissionWildcard})},
			true,
		},
		{
			"a role granting nothing is not",
			dtos.RoleRow{Found: true},
			false,
		},
		{
			"an UNKNOWN role is, fail-closed",
			dtos.RoleRow{Found: false},
			true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.GrantsWildcard(); got != c.want {
				t.Errorf("grantsWildcard() = %v, want %v", got, c.want)
			}
		})
	}
}

// ── unusable ids never reach the store ──────────────────────────────────────

// Defence in depth for the 500 this service is one half of. The domain refuses
// an unusable id before asking, but if that guard is ever weakened, THIS is the
// seat that binds the value into a criterion against a UUID column and panics.
//
// A nil repository is what proves it: reaching the store here would panic on the
// nil rather than return.
func TestAnUnusableTenantIsAnsweredWithoutTouchingTheGroupStore(t *testing.T) {
	svc := &GroupServiceImpl{}
	for _, id := range []string{"", "tatu"} {
		if !svc.TenantIsUnavailable(domain.NewID(id)) {
			t.Errorf("%q was reported as an available tenant", id)
		}
	}
}

func TestAnUnusableRoleIDResolvesToNotFoundWithoutTouchingTheStore(t *testing.T) {
	svc := &GroupServiceImpl{}

	// ASKED AS A SET, which is what the batched resolver takes — and a set of
	// nothing but unusable ids must still reach no store: the read closure runs
	// only for ids that parse, so a nil repository is never dereferenced.
	unusable := []domain.ID{domain.NewID(""), domain.NewID("tatu")}
	rows := svc.roleRows(unusable)
	for _, id := range unusable {
		row := rows[id]
		if row.Found {
			t.Errorf("%q resolved to a role", id.String())
		}
		if !row.GrantsWildcard() {
			t.Errorf("%q did not fail closed on the wildcard question", id.String())
		}
	}
}

// The same guard seen through the two public facts, which is how the rules
// actually reach it.
func TestTheThreePerEntryFactsAllShortCircuitAnUnusableRole(t *testing.T) {
	svc := &GroupServiceImpl{}
	unusable := domain.NewID("tatu")
	set := []domain.ID{unusable}

	if !svc.RoleIsUnavailableInTenant(domain.NewID("0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410"), set)[unusable] {
		t.Error("an unusable role reference was reported as available")
	}
	if !svc.RoleGrantsWildcard(set)[unusable] {
		t.Error("an unusable role reference did not fail closed on the wildcard question")
	}
}

// ── the identity paths ──────────────────────────────────────────────────────

// The dev-profile half of the two-state table, at the SERVICE seat this time.
// An absent identity stands down; an identity that is PRESENT and lacks a
// permission refuses. Collapsing the two makes the entity unusable on a bench
// that issues no tokens.
func TestCallerLacksAnyPermissionOfStandsDownWithoutAnIdentity(t *testing.T) {
	attached := []domain.ID{domain.NewID(probedRoleID)}

	// An empty answer, not a map of falses: an absent key is the fact answering
	// NOTHING for that entry, which the rule reads as the zero value and does
	// not raise.
	//
	// No bound context at all — outside a request entirely.
	if len((&GroupServiceImpl{}).CallerLacksAnyPermissionOf(attached)) != 0 {
		t.Error("the escalation probe answered a call made outside any request")
	}

	// A bound context that carries no identity: auth.mode disabled.
	ctx := configuration.NewAppContextWithRandomID(configuration.LangENG)
	if len((&GroupServiceImpl{ctx: ctx}).CallerLacksAnyPermissionOf(attached)) != 0 {
		t.Error("the escalation probe answered a request whose context carries no identity")
	}
}

// A *:* holder passes for every concrete permission, so the transitive rule
// needs no super-admin branch of its own. This is what says that is true rather
// than assumed — and it reaches no store, because the memo is primed first.
func TestASuperAdminLacksNothingInAConcreteBundle(t *testing.T) {
	ctx := configuration.NewAppContextWithRandomID(configuration.LangENG)
	ctx.SetIdentity(&configuration.Identity{
		Claims: map[string]any{"permissions": []any{"*:*"}},
	})
	svc := &GroupServiceImpl{ctx: ctx}

	// Prime the request memo so the fact answers from it and never queries.
	roleID := domain.NewID(probedRoleID)
	ctx.Set(groupRoleMemoPrefix+roleID.String(), dtos.RoleRow{
		Found: true,
		Keys:  concreteKeys([2]string{"tenant", "read"}, [2]string{"role", "insert"}, [2]string{"group", "grant"}),
	})

	if svc.CallerLacksAnyPermissionOf([]domain.ID{roleID})[roleID] {
		t.Error("a *:* super-admin was refused a role granting only concrete permissions")
	}
}

// The TRANSITIVE half: every key must be held, and the first one that is not
// refuses the attachment. A caller holding two of three permissions is refused.
func TestACallerMissingOneKeyOfTheBundleIsRefused(t *testing.T) {
	ctx := configuration.NewAppContextWithRandomID(configuration.LangENG)
	ctx.SetIdentity(&configuration.Identity{
		Claims: map[string]any{"permissions": []any{"tenant:read", "role:insert"}},
	})
	svc := &GroupServiceImpl{ctx: ctx}

	roleID := domain.NewID(probedRoleID)
	held := dtos.RoleRow{Found: true, Keys: concreteKeys([2]string{"tenant", "read"}, [2]string{"role", "insert"})}
	ctx.Set(groupRoleMemoPrefix+roleID.String(), held)
	if svc.CallerLacksAnyPermissionOf([]domain.ID{roleID})[roleID] {
		t.Fatal("a caller holding every key of the bundle was refused")
	}

	// One more key, which the caller does not hold. Nothing else changes.
	other := domain.NewID("0198f400-1111-7000-8000-aaaaaaaaaaaa")
	ctx.Set(groupRoleMemoPrefix+other.String(), dtos.RoleRow{
		Found: true,
		Keys:  concreteKeys([2]string{"tenant", "read"}, [2]string{"role", "insert"}, [2]string{"group", "grant"}),
	})
	if !svc.CallerLacksAnyPermissionOf([]domain.ID{other})[other] {
		t.Error("a caller was allowed to confer a role granting a permission they do not hold")
	}
}

// The second lock behind the wildcard rule. A wildcard key must never be handed
// to Identity.HasPermission — it panics on one — so the fact answers "lacks"
// without calling through. A panic on a security rule is a 500 on exactly the
// path that rule exists to close.
func TestAWildcardBearingBundleIsRefusedWithoutAskingTheIdentity(t *testing.T) {
	ctx := configuration.NewAppContextWithRandomID(configuration.LangENG)
	ctx.SetIdentity(&configuration.Identity{
		Claims: map[string]any{"permissions": []any{"*:*"}},
	})
	svc := &GroupServiceImpl{ctx: ctx}

	roleID := domain.NewID(probedRoleID)
	ctx.Set(groupRoleMemoPrefix+roleID.String(), dtos.RoleRow{
		Found: true,
		Keys:  concreteKeys([2]string{vos.PermissionWildcard, vos.PermissionWildcard}),
	})

	// Even for a super-admin: the answer comes from the guard, not the claim.
	if !svc.CallerLacksAnyPermissionOf([]domain.ID{roleID})[roleID] {
		t.Error("a wildcard-bearing bundle reached the identity instead of the guard")
	}
}

// The memo is per REQUEST. Two services bound to two contexts must not see each
// other's rows — a shared cache of role bundles would answer a security rule
// with another request's state.
func TestTheRoleMemoIsScopedToOneRequest(t *testing.T) {
	roleID := domain.NewID(probedRoleID)

	first := configuration.NewAppContextWithRandomID(configuration.LangENG)
	first.Set(groupRoleMemoPrefix+roleID.String(), dtos.RoleRow{Found: true})

	if _, ok := first.Get(groupRoleMemoPrefix + roleID.String()); !ok {
		t.Fatal("the memo did not store the row on its own context")
	}
	second := configuration.NewAppContextWithRandomID(configuration.LangENG)
	if _, ok := second.Get(groupRoleMemoPrefix + roleID.String()); ok {
		t.Error("a second request saw the first request's memoised role")
	}
}
