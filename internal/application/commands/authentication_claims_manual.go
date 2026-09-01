// Hand-written, and not a hook: no generator declares this file.
//
// THE TWO-LEVEL CLAIM CHAIN, resolved into the map a token carries. It is a
// file of its own rather than three more functions in
// authentication_commands_manual.go for the same reason the journal is:
// everything here is a pure function of a loaded aggregate and a slice of
// definitions, so it is unit-testable without a database, a signing key or a
// handler, and the handlers are left saying WHEN it runs rather than what it
// decides.
//
// The chain, and it is ordered rather than merged — first non-null wins:
//
//	user_claims.value        level 1 — the value set on this user
//	      ↓ if absent
//	claims.default_value     level 2 — the tenant-wide default of the
//	                                   definition that owns the name
//	      ↓ if null
//	the claim does not enter the token at all
//
// An ABSENT claim and an EMPTY one are not the same thing to a consumer, so
// nothing here ever mints a zero value to fill a hole.
//
// WHY THE WALK IS OVER THE CATALOG AND NOT OVER THE USER'S ENTRIES. Level 1
// arrives free — the repository's read join fills ClaimName and ClaimValueType
// on every loaded entry — and iterating those entries would answer the
// specialised half with no query at all. It is still the wrong anchor twice
// over: the defaults of level 2 belong to definitions this user holds no entry
// for, so they would never be reached; and a read join is not archive-gated on
// its target, so an entry whose definition was retired would keep minting. The
// catalog is the vocabulary, the entries are an overlay on it.

package commands

import (
	"log/slog"
	"math"
	"sort"
	"strconv"

	appdomain "github.com/ClaudioSchirmer/authcore/internal/domain"
	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
)

// customClaimBudget is how many tenant-defined claims one token may carry.
//
// A HEADER BUDGET BEFORE IT IS A COUNT. A claim value caps at 256 runes, so
// twenty of them is about 5 KB riding on every request to every service —
// inside the 8 KB buffer most proxies default to, and on top of a fixed claim
// set that was already argued down to keys instead of display names.
//
// Twenty is deliberately the same number the write side already enforces per
// principal (the claims-cap rule on User), so a user holding the maximum number
// of specialised values loses none of them to this cap — see the ordering in
// resolveCustomClaims.
//
// It is a CONSTANT AND NOT A CONFIG KEY. A deployment able to raise it would be
// able to change what consuming services authorize on, one profile at a time,
// with no audit trail and no review — and one able to lower it would silently
// drop claims that a consumer had already come to depend on.
//
// WHAT THIS CAP DOES NOT FIX, stated here because the seam is easy to mistake
// for a solution: the catalog itself has no ceiling on definitions per tenant,
// so a tenant CAN create more claims carrying defaults than a token can hold.
// The loud answer is to refuse that definition at the moment it is created,
// which is a change to the Claim aggregate and a run of its own. Until then
// this is the quiet answer, and it truncates rather than refuses because
// truncation is the fail-closed direction here: a custom claim only ever ADDS a
// fact, so dropping one can deny a consumer and can never grant.
const customClaimBudget = 20

// platformClaimNames is the fixed set this service mints itself.
//
// Built from the constants rather than from a second list of literals: a claim
// renamed in one place and not the other would silently reopen the hole this
// set exists to close.
//
// IT IS A SEATBELT, NOT THE MECHANISM. The mechanism is vos.ClaimName's
// reserved x_ prefix: every definition created through the API carries it, no
// platform claim does, so a collision cannot be written in the first place. The
// check stays because the catalog can also be written by migration — the door
// the platform's own nine would enter by — and because a claim that overwrote
// `permissions` or `tenant_id` would not merely confuse one consumer, it would
// change what every service in the mesh authorizes.
var platformClaimNames = map[string]struct{}{
	claimIdentityKind:       {},
	claimTenantID:           {},
	claimTenantWorkspace:    {},
	claimEmail:              {},
	claimName:               {},
	claimPermissions:        {},
	claimGroups:             {},
	claimRoles:              {},
	claimMustChangePassword: {},
}

// resolvedClaim is one name and the typed value that reached it.
//
// Unexported and local to the ordering: the budget has to spend specialised
// values before defaults, and a map cannot be ordered. Nothing outside this
// file sees it.
type resolvedClaim struct {
	name  string
	value any
}

// resolveCustomClaims walks the chain and returns what the token should carry
// beside the fixed set.
//
// ALWAYS NON-NIL, so both readers — the token and the response body — get a map
// they can range over and a body field that renders `{}` rather than `null`.
//
// A MUST-CHANGE-PASSWORD SESSION GETS NOTHING, and that is the same reading
// that already governs the permissions claim one file over: a session that
// exists to rotate an expired credential carries nothing but the one thing it
// is for. It costs nothing to hold that line — the change-password route is
// this service's own and reads no custom claim — and the alternative is a token
// with no authority carrying tenant facts a consumer might still branch on.
func resolveCustomClaims(account *schemas.SignInAccount, bundle infra.SignInBundle) map[string]any {
	resolved := map[string]any{}
	if account == nil || account.MustChangePassword {
		return resolved
	}

	held := bundle.ClaimValues

	// Two buckets rather than one, because the budget spends them in order.
	var specialised, defaulted []resolvedClaim
	for _, definition := range bundle.Definitions {
		name := definition.Name
		if _, reserved := platformClaimNames[name]; reserved {
			// Not a refusal to the caller: the sign-in has exactly ONE refusal
			// and a catalog problem must not become a second. The operator who
			// seeded this row is the audience, and the log is where they are.
			slog.Default().Warn("token.claim.platform_name_skipped",
				slog.String("claim", name))
			continue
		}

		value, fromUser := "", false
		if v, ok := held[definition.ID]; ok {
			value, fromUser = v, true
		} else if definition.DefaultValue != nil {
			value = *definition.DefaultValue
		} else {
			// Null at both levels. The claim is ABSENT — not empty, not zero.
			continue
		}

		typed, ok := typedClaimValue(vos.ClaimValueType(definition.ValueType), value)
		if !ok {
			// Unreachable through the API — both levels validate against this
			// same value type on the way in — and reachable by a migration or a
			// direct UPDATE. Omitted rather than coerced: minting "abc" into a
			// claim a consumer reads as a number is a wrong answer, and no
			// answer is the honest one.
			slog.Default().Warn("token.claim.unreadable_value_skipped",
				slog.String("claim", name),
				slog.String("valueType", definition.ValueType))
			continue
		}

		entry := resolvedClaim{name: name, value: typed}
		if fromUser {
			specialised = append(specialised, entry)
		} else {
			defaulted = append(defaulted, entry)
		}
	}

	// SPECIALISED FIRST, then defaults, each half by name. The order is the
	// budget's policy and it is deliberate in both halves: a value somebody set
	// on this user must never be crowded out by a tenant-wide default, and
	// sorting by name inside each half makes the truncation reproducible — the
	// same user with the same rows always loses the same claims, so "why did
	// x_zone disappear" has an answer.
	sortClaimsByName(specialised)
	sortClaimsByName(defaulted)

	ordered := make([]resolvedClaim, 0, len(specialised)+len(defaulted))
	ordered = append(ordered, specialised...)
	ordered = append(ordered, defaulted...)

	var dropped []string
	for _, entry := range ordered {
		if len(resolved) >= customClaimBudget {
			dropped = append(dropped, entry.name)
			continue
		}
		resolved[entry.name] = entry.value
	}
	if len(dropped) > 0 {
		// NAMED, not counted. "Twelve claims were dropped" tells an operator
		// they have a problem; the names tell them which consumer broke.
		slog.Default().Warn("token.claim.budget_exceeded",
			slog.Int("budget", customClaimBudget),
			slog.Int("droppedCount", len(dropped)),
			slog.Any("dropped", dropped))
	}
	return resolved
}

// typedClaimValue renders a stored string as the JSON type its definition
// declares, or reports that it cannot.
//
// THE GATE IS THE DOMAIN'S OWN FUNCTION, deliberately. ClaimValueMatchesValueType
// is what both write levels ask, so calling it here means the token can never
// disagree with the two rules that let the value in — there is no fourth
// reading of what a bool is, and a value the catalog accepted as a default can
// never be one this refuses to mint.
//
// Typing at all is why the read join carries ClaimValueType up to the entry: a
// stored "true" is unreadable without it, and a consumer branching on
// x_seat_count needs a number rather than a string it has to parse. valueType
// is immutable on the definition, so a claim's JSON type never changes under a
// consumer that already depends on it.
//
// number is float64 because that is what the domain means by number — the
// write-side check is ParseFloat, and a second definition here would be exactly
// the drift the shared gate exists to prevent.
func typedClaimValue(valueType vos.ClaimValueType, value string) (any, bool) {
	if !appdomain.ClaimValueMatchesValueType(valueType, value) {
		return nil, false
	}
	switch valueType {
	case vos.ClaimValueTypeString:
		return value, true

	case vos.ClaimValueTypeNumber:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			// The gate above already refused all three, so this is agreement
			// rather than a second check — and it is here because dropping the
			// error would mean minting 0 for a value that did not parse.
			return nil, false
		}
		return parsed, true

	case vos.ClaimValueTypeBool:
		// Exactly the two spellings the gate admits; anything else never
		// reaches this line.
		return value == "true", true

	default:
		// The Unknown sentinel — a value_type outside the closed set, which no
		// write can produce and a migration can. The shared gate answers TRUE
		// for it, because on the write path the value object raises its own
		// notification straight after; there is no value-object pass here, so
		// this is the seat that has to decide, and it omits rather than mint a
		// value whose type nobody declared.
		return nil, false
	}
}

// sortClaimsByName orders one bucket, so truncation is reproducible.
func sortClaimsByName(entries []resolvedClaim) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
}
