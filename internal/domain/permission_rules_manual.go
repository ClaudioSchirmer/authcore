// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Permission (1 to implement).
//
// entity:     Permission
// spec:       specs/omnicore-gen/permission.omnicore.yaml
// generator:  omnicore-gen (created this file, does not maintain it)
// created:    2026-08-20
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
func (e *Permission) customRules(actionName string, service domain.Service, r *domain.Rules) {
	r.IfInsertOrUpdate(func() {
		// ── description-differs-from-key ──
		// The Description must differ from the rendered resource:action
		// string under a normalized comparison — case-folded, with
		// whitespace, colons and hyphens collapsed — which catches the
		// lazy paste and nothing more.
		//
		// The key is rendered through Key.String(), never by joining the two
		// parts here: that method is the one place in this service that knows
		// the separator is a colon, and a second join is a second definition
		// that can drift.
		//
		// The comparison is normalized rather than literal so that "tenant:read"
		// pasted as "Tenant Read" is caught too. A permission's description is
		// meant to say what holding it LETS A CALLER DO; one that merely echoes
		// the key tells an operator nothing they could not already read.
		//
		// The empty case belongs to the value object, not here — vos.Description
		// answers an empty value with RequiredFieldNotification on this same
		// pass, and a second complaint about the same field would be noise.
		if description := normalizeKeyForComparison(e.Description.Value()); description != "" {
			if description == normalizeKeyForComparison(e.Key.String()) {
				r.AddNotification("Description", PermissionDescriptionEchoesKeyNotification{})
			}
		}
	})
}

// normalizeKeyForComparison folds a value to the form the "must differ" rule
// compares on: the shared normalization (lower-cased, whitespace and hyphens
// removed) with colons dropped as well, so that the rendered key and a prose
// paste of it collapse to the same string.
//
// The colon is stripped HERE rather than in normalizeForComparison because that
// helper is shared with Tenant, whose rule has no key to compare against and
// no reason to change behaviour.
//
// It is a COMPARISON helper only — nothing is ever persisted in this form. This
// service never repairs a value it was given.
func normalizeKeyForComparison(s string) string {
	return normalizeForComparison(strings.ReplaceAll(s, ":", ""))
}
