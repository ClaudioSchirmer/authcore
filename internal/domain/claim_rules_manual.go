// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Claim (2 to implement).
//
// entity:     Claim
// spec:       specs/omnicore-gen/claim.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-28
//
// Both are implemented below. One design note that is load-bearing:
//
//	GATE ORDER, AND WHY tenant-must-exist DOES NOT TRUST ITS ARGUMENT.
//	The framework dispatches gates in a fixed order — every IfInsert clause
//	runs before every IfInsertOrUpdate one, whatever the declaration order —
//	so this file's IfInsert block runs BEFORE the generated
//	`tenant-is-a-usable-id` barrier, not after it. TenantIsUnavailable can
//	therefore be handed an empty or unparseable id, and that is where the
//	fail-closed reading lives: the implementation in internal/infra answers
//	"unavailable" for an id that is not a usable UUID, rather than querying
//	with it. Role's identically-scoped rule carries the same guard, for the
//	same reason.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package domain

import (
	"math"
	"strconv"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// customRules is called at the end of the generated BuildRules, with the same
// arguments, and reports a violation the same way: r.AddNotification.
//
// ONE gate per verb, holding every rule that runs on that verb — the shape
// the generated BuildRules already has.
func (e *Claim) customRules(actionName string, service domain.Service, r *domain.Rules) {
	r.IfInsert(func() {
		// ── tenant-must-exist ──
		// The owning tenant must exist, must not be archived and must not be
		// commercially SUSPENDED. A TRIAL tenant is a live customer and passes.
		//
		// Insert only, and that is deliberate rather than an omission: a tenant
		// suspended AFTER a definition was created must not make that
		// definition impossible to correct. The same call Role makes.
		//
		// A read join cannot answer this. The traversal into Tenant IS declared
		// on the repository — so TenantStatus is an ordinary field of this
		// entity on every LOAD — but an insert has no row to load, so the field
		// is blank exactly where this rule fires.
		if service.(ClaimService).TenantIsUnavailable(e.TenantID) {
			r.AddNotification("TenantID", ClaimTenantDoesNotExistNotification{})
		}
	})

	r.IfUpdate(func() {
		e.refuseNarrowingAppliesToWithHeldValues(service.(ClaimService), r)
	})

	r.IfInsertOrUpdate(func() {
		// ── default-value-matches-value-type ──
		// The default must parse as the declared ValueType. A NULL default is
		// ALWAYS valid: "no default" is a legitimate state — level 2 of the
		// chain simply does not fire — and an absent claim is not the same
		// thing as an empty one to a consumer.
		if e.DefaultValue == nil {
			return
		}
		if !ClaimValueMatchesValueType(e.ValueType, *e.DefaultValue) {
			r.AddNotification("DefaultValue", DefaultValueDoesNotMatchValueTypeNotification{}, *e.DefaultValue)
		}
	})
}

// ClaimValueMatchesValueType reports whether a value parses as the type a claim
// definition declares.
//
// EXPORTED, and it serves BOTH LEVELS OF THE CHAIN. Level 2 is the caller right
// above — claims.default_value, judged by this entity's own rule. Level 1 is
// user_claims.value and client_claims.value, judged out in internal/infra by
// the ClaimValueDoesNotMatchValueType fact each parent's service adapter
// implements, which calls this same function after reading the definition's
// value_type.
//
// One function rather than two identical switches, deliberately: "the same
// three readings at both levels" is a promise the chain depends on — two levels
// disagreeing about what a bool is would mean a value the catalog accepted as a
// default is refused as a specialised value, or the reverse — and a second copy
// is a rule that can drift from the first.
//
// It switches on the ENUM MEMBER rather than on the raw string, which is what
// makes the last branch correct instead of merely permissive: a ValueType
// outside the closed set converges to the Unknown sentinel, and the framework's
// automatic value-object pass already answers that with
// UnknownClaimValueTypeNotification at the end of the rules. Refusing here too
// would tell the caller about one bad field twice, in two different sentences.
//
// NOTHING IS TRIMMED, matching the rest of this entity: " 1000" is not a
// number, and repairing it here would store a value the caller did not send.
func ClaimValueMatchesValueType(valueType vos.ClaimValueType, value string) bool {
	switch valueType {
	case vos.ClaimValueTypeString:
		// Non-empty. At level 2 that is because an empty default is not "no
		// default" — NULL is what says that, and the two reach a consumer
		// differently. At level 1 the ClaimValue value object already refuses
		// an empty value before this is asked, so the branch is agreement
		// rather than a second gate.
		return value != ""

	case vos.ClaimValueTypeNumber:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		// ParseFloat accepts "NaN", "Inf" and "-Inf". None of them is a number
		// a consuming service can branch on, and all three would ride a token
		// as literal text, so they are refused explicitly rather than inherited
		// from the parser's vocabulary.
		return !math.IsNaN(parsed) && !math.IsInf(parsed, 0)

	case vos.ClaimValueTypeBool:
		// EXACTLY these two, not strconv.ParseBool — which also accepts "1",
		// "t", "TRUE" and "False". A claim value is read by services this one
		// does not control, so the token carries one spelling per truth value.
		return value == "true" || value == "false"

	default:
		// The Unknown sentinel: already refused by the value object. See the
		// doc comment above.
		return true
	}
}

// refuseNarrowingAppliesToWithHeldValues is the other half of
// value-type-immutable: the second way an edit to this row can retro-invalidate
// values already stored on the two edge collections.
//
// AppliesTo is mutable, and deliberately so — widening is the ordinary
// operational move, a definition that started `user` becoming `both` the day
// the machine side of an integration arrives. But the field is mutable in BOTH
// directions, and narrowing is not symmetric with widening: narrowing `both` to
// `user` while client_claims rows hold values for this definition STRANDS them.
// They stay in the table, they stay readable on GET /clients/:id, and they would
// be refused by claim-applies-to-client if anybody tried to write them. Nothing
// complains, and without this rule nothing ever would.
//
// SIX transitions are possible and FOUR of them narrow — both->user and
// both->client drop one kind each, and user->client and client->user drop one
// each as well. The cross pair is the one a rule written as "did it lose Both?"
// would miss, which is why this asks what the NEW value admits rather than what
// the old one was.
//
// A definition nobody holds a value for narrows freely, and that has to keep
// working: refusing every narrowing would make the field effectively immutable,
// which the model gate decided the other way.
//
// ARCHIVE is deliberately not covered. Archiving a definition while principals
// hold values is an ALREADY-ACCEPTED state: archive is one-way here, a retired
// definition comes back as a NEW row with a NEW id precisely so an edge holding
// the old id cannot silently re-attach, and writes against an archived
// definition are already refused by claim-available-in-tenant on both parents.
// Narrowing is different because it leaves the definition LIVE and the values
// INVISIBLE — the row is neither refused nor retired, it just quietly stops
// meaning anything.
func (e *Claim) refuseNarrowingAppliesToWithHeldValues(service ClaimService, r *domain.Rules) {
	if service == nil {
		return
	}

	// domain.Old is the pre-write snapshot, and the same accessor the three
	// generated immutability rules above read. It is nil on an insert, which
	// this gate never sees, and on an update the framework could not load —
	// there is nothing to compare against either way.
	old := domain.Old(e)
	if old == nil {
		return
	}

	// Staying put asks nothing. Neither does a widening — and the two are the
	// same test, because "still admits everything it used to" covers both.
	if old.AppliesTo == e.AppliesTo {
		return
	}

	// The ENUM MEMBER, not the raw string: a value outside the closed set
	// converges to the Unknown sentinel, which the framework's automatic
	// value-object pass already answers with UnknownClaimAppliesToNotification.
	// Reading the raw string here would refuse the same bad field twice, in two
	// different sentences.
	dropsUsers := ClaimAdmitsUsers(old.AppliesTo) && !ClaimAdmitsUsers(e.AppliesTo)
	dropsClients := ClaimAdmitsClients(old.AppliesTo) && !ClaimAdmitsClients(e.AppliesTo)

	// Ask ONLY about the kinds this change actually drops. A widening reaches
	// neither branch and queries nothing at all, which is the property the
	// tests assert on the stub's call count rather than on the outcome: a
	// widening that queries two tables is a correctness bug no assertion about
	// the answer would catch.
	name := e.Name.Value()
	if dropsUsers && service.ClaimIsHeldByAUser(e.TenantID, name) {
		r.AddNotification("AppliesTo", ClaimAppliesToCannotExcludeHeldValuesNotification{}, e.AppliesTo)
		return
	}
	if dropsClients && service.ClaimIsHeldByAClient(e.TenantID, name) {
		r.AddNotification("AppliesTo", ClaimAppliesToCannotExcludeHeldValuesNotification{}, e.AppliesTo)
	}
}

// ClaimAdmitsUsers and ClaimAdmitsClients read one enum member as the two
// questions the chain actually asks of it.
//
// EXPORTED, and shared by three callers rather than inlined at each: the
// narrowing guard above compares old against new for each kind, and each
// parent's ClaimDoesNotApplyTo… fact in internal/infra asks the same question of
// the definition an entry points at. Written out five times it is a comparison
// that can be written wrong once — and the failure would be silent in the worst
// direction, letting a client hold a user-only claim.
//
// The Unknown sentinel admits NOBODY, by falling through both: a value outside
// the closed set is not a permission to hold anything.
func ClaimAdmitsUsers(v vos.ClaimAppliesTo) bool {
	return v == vos.ClaimAppliesToUser || v == vos.ClaimAppliesToBoth
}

func ClaimAdmitsClients(v vos.ClaimAppliesTo) bool {
	return v == vos.ClaimAppliesToClient || v == vos.ClaimAppliesToBoth
}
