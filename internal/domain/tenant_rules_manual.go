// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Tenant (4 to implement).
//
// entity:     Tenant
// spec:       omnicore-gen/tenant.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-19
//
// Until these are written the service runs and accepts writes — it simply
// does not enforce the invariants the spec described. That is quiet, which
// is exactly why it is worth doing now rather than later.
//
// There is no checksum here on purpose: this file exists to be edited, so
// hashing it would report drift every time you did the thing it is for.

package domain

import (
	"strings"
	"unicode"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// customRules is called at the end of the generated BuildRules, with the same
// arguments, and reports a violation the same way: r.AddNotification.
//
// ONE gate per verb, holding every rule that runs on that verb — the shape
// the generated BuildRules already has. A gate per rule reads as a wall of
// near-identical closures and makes the framework dispatch the same verb once
// per rule on every write; rules that share a verb belong in the same block. A
// rule that appears under two gates is still ONE rule: write it as a method
// and call it from both, rather than as two copies that can drift apart.
func (e *Tenant) customRules(actionName string, service domain.Service, r *domain.Rules) {

	r.IfInsert(func() {
		// ── derive-tenant-id ──
		// On insert, set TenantID to Workspace.DeriveTenantID() — the
		// UUIDv5 of the service's TenantIDNamespace over the workspace
		// handle. The field is server-derived and reaches no write DTO, so
		// this is the only place the value is ever produced.
		//
		// It is an assignment and not a validation, so it raises nothing.
		// A malformed or empty handle is refused by the value object on this
		// same pass; deriving from one would only produce a value the write
		// never commits, so the empty case is skipped rather than guarded
		// with a second complaint.
		//
		// This runs BEFORE the IfInsertOrUpdate block below, because the
		// framework executes each gate in declaration order — which is what
		// lets the check there see the value assigned here.
		if e.Workspace != "" {
			e.TenantID = e.Workspace.DeriveTenantID()
		}
	})

	r.IfInsertOrUpdate(func() {
		// ── tenant-id-matches-workspace ──
		// TenantID must equal Workspace.DeriveTenantID(). The value is
		// computed by the rule above, so this can only fail through a bug in
		// that derivation or a hand-written row — which is exactly what it
		// guards.
		//
		// On update neither field is a member of the request body, so both
		// carry what the row held; a mismatch here means the row itself is
		// inconsistent and must not be written back.
		if e.Workspace != "" && e.TenantID != e.Workspace.DeriveTenantID() {
			r.AddNotification("TenantID", TenantIDDerivationMismatchNotification{})
		}

		// ── description-differs-from-name-and-workspace ──
		// The description must differ from both Name and Workspace under a
		// normalized comparison — case-folded, with whitespace and hyphens
		// collapsed — which catches the pasted-name description.
		//
		// The comparison is normalized rather than literal so that "Acme
		// Comércio" pasted as "acme-comercio" is caught too. The empty case
		// belongs to the value object, not here.
		if description := normalizeForComparison(e.Description.Value()); description != "" {
			if description == normalizeForComparison(e.Name.Value()) ||
				description == normalizeForComparison(e.Workspace.Value()) {
				r.AddNotification("Description", TenantDescriptionMustDifferNotification{})
			}
		}
	})

	r.IfArchive(func() {
		// ── archive-forces-suspended ──
		// Archiving forces Status to suspended. This is a MUTATION, not a
		// validation: set the field and raise nothing. An archived tenant is
		// never commercially active, so archived+active becomes an
		// unrepresentable state, and unarchiving brings the tenant back
		// suspended by consequence.
		//
		// Three things follow, and none of them is an oversight:
		//
		//   * It never violates the status-transition rule. Every move it can
		//     cause — trial→suspended, active→suspended, suspended→suspended —
		//     is allowed, and the two rules could not meet anyway: the
		//     transition is gated IfUpdate and this is IfArchive.
		//   * Unarchiving returns the tenant SUSPENDED, because the stored
		//     value IS suspended — archiving wrote it. Restoring a tenant must
		//     never silently resume a billable, functioning account;
		//     reactivation is a separate, separately audited act.
		//   * It is one-way. Setting a tenant active does NOT unarchive it.
		//
		// This reaches the ROW only from framework v0.54.0 onward, where
		// archive stopped being a lone deleted_at UPDATE and began writing the
		// full field set like every other verb. At v0.53.0 the same line
		// reached the audit event and the outbox payload and never the row.
		e.Status = vos.TenantStatusSuspended
	})

	_ = actionName
	_ = service
	_ = r
}

// normalizeForComparison folds a value to the form the "must differ" rule
// compares on: lower-cased, with every hyphen and whitespace rune removed.
//
// It is a COMPARISON helper only — nothing is ever persisted in this form.
// Rule 14 of the model is explicit that this service never repairs a value it
// was given.
func normalizeForComparison(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
