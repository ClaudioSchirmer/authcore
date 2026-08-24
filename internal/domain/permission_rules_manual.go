// THIS FILE IS YOURS. Written once by omnicore-gen, never again.
//
// The hand-written rules for Permission (1 to implement).
//
// entity:     Permission
// spec:       specs/omnicore-gen/permission.omnicore.yaml
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
		// ── description-does-not-echo-key ──
		// The description must differ from the rendered resource:action
		// string under a normalized comparison — case-folded, with
		// whitespace, colons and hyphens collapsed — which catches the
		// lazy paste and nothing more. Render the key through
		// PermissionKey.String(); do not concatenate the parts here, the
		// value object is the single home of the separator.
		//
		// It catches the lazy paste and nothing more: a description that
		// merely restates the key teaches an operator nothing the listing
		// did not already show them.
		if e.Description == "" {
			// An empty description is the Description value object's
			// complaint to make, not this rule's. Saying it twice would have
			// the caller read one problem in two places.
			return
		}
		if normalizeForEcho(e.Description.Value()) == normalizeForEcho(e.Key.String()) {
			r.AddNotification("Description", PermissionDescriptionEchoesKeyNotification{}, e.Description.Value())
		}
	})
}

// normalizeForEcho reduces a value to what the echo comparison is actually
// about: the letters and digits it carries, case-folded.
//
// Whitespace, colons, hyphens, underscores and dots are dropped rather than
// collapsed to a space, which is what makes "tenant:read", "Tenant Read" and
// "tenant-read" the same value to this rule while leaving any real sentence
// about the permission comfortably different.
//
// DELIBERATELY NOT normalizeForComparison, the Tenant rule's fold in this same
// package, and the difference is not an oversight — it was raised and kept.
// That one collapses whitespace and hyphens to a single space and PRESERVES
// punctuation, which is right when the other side of the comparison is a
// human handle. Here the other side is a KEY that contains the separator, so
// "tenant:read" would keep its colon and never match "TENANT READ". What the
// two rules compare against differs, so how they fold differs; neither is the
// general case of the other.
func normalizeForEcho(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
