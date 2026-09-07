// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Tenant (4 to implement).
//
// entity:     Tenant
// spec:       specs/omnicore-gen/tenant.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-24
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
	r.IfInsertOrUpdate(func() {
		// ── description-differs-from-name-and-workspace ──
		// The description must differ from both Name and Workspace under a
		// normalized comparison — case-folded, with whitespace and hyphens
		// collapsed — which catches the pasted-name description.
		description := normalizeForComparison(e.Description.Value())
		if description != "" &&
			(description == normalizeForComparison(e.Name.Value()) ||
				description == normalizeForComparison(e.Workspace.Value())) {
			r.AddNotification(&e.Description, TenantDescriptionMustDifferNotification{}, true)
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
		// It reaches the ROW, not merely the audit event, because archive is an
		// ordinary full-field write at this pin: it emits the same UPDATE every
		// other verb does, with archived_at riding along as one more column.
		//
		// It cannot collide with the status transition rule: that rule is
		// IfUpdate, this is IfArchive, and the two modes never run together.
		e.Status = vos.TenantStatusSuspended
	})
}

// normalizeForComparison folds a value to the form the description rule
// compares on: lowercased, with every run of whitespace and hyphens collapsed
// to a single space, and trimmed.
//
// It exists so that "acme-comercio" and "Acme  Comercio" are recognised as the
// same text, which is what makes the pasted-name description detectable. It is
// deliberately NOT accent-folding: the rule is about someone pasting a value,
// not about near-matches, and folding accents would start refusing legitimate
// descriptions that merely resemble the name.
//
// Nothing here is ever stored — this is a comparison form, and the values
// themselves are refused rather than repaired.
func normalizeForComparison(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	pendingSeparator := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || r == '-' {
			pendingSeparator = true
			continue
		}
		if pendingSeparator && b.Len() > 0 {
			b.WriteRune(' ')
		}
		pendingSeparator = false
		b.WriteRune(r)
	}
	return b.String()
}
