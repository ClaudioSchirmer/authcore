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

	r.IfInsertOrUpdate(func() {
		// ── default-value-matches-value-type ──
		// The default must parse as the declared ValueType. A NULL default is
		// ALWAYS valid: "no default" is a legitimate state — level 2 of the
		// chain simply does not fire — and an absent claim is not the same
		// thing as an empty one to a consumer.
		if e.DefaultValue == nil {
			return
		}
		if !defaultValueMatchesValueType(e.ValueType, *e.DefaultValue) {
			r.AddNotification("DefaultValue", DefaultValueDoesNotMatchValueTypeNotification{}, *e.DefaultValue)
		}
	})
}

// defaultValueMatchesValueType reports whether a default parses as the type the
// definition declares.
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
func defaultValueMatchesValueType(valueType vos.ClaimValueType, value string) bool {
	switch valueType {
	case vos.ClaimValueTypeString:
		// Non-empty, because an empty default is not "no default" — that is
		// what NULL is for, and the two reach a consumer differently.
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
