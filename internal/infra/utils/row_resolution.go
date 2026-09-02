// Hand-written, and deliberately NOT named for an entity.
//
// This file holds what "resolve a SET of rows, once" MEANS, and it lives on its
// own because four aggregates ask it: User, Client, Group and Role, each about
// a different companion table. It started inside user_service_manual.go and
// moved here the day the second caller appeared — the same move dtos.RoleRow made
// into role_probe.go, and for the same reason: a batch helper hanging off a
// file named for User would mean that removing User breaks Client for a reason
// that has nothing to do with users.
//
// What is NOT here is any resolution: the reads, the memo prefixes and the
// repositories stay on each service, because those are per-service concerns.
// What is shared is the arithmetic of a batch — dedup, the canonical key, the
// parse guard, and what an id the read did not answer for means.

package utils

import (
	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// ResolveRows answers a whole requested set from ONE read.
//
// It is the shared half of every batched resolver in this package, and
// everything about a batch that is easy to get wrong lives here rather than
// once per aggregate:
//
//   - THE MEMO IS KEYED BY THE CANONICAL ID, the answer by the id the caller
//     PASSED. uuid.Parse accepts spellings the store renders one way — upper
//     case, braces, a urn prefix — so a raw-string memo key would read one row
//     twice, and a canonically-keyed ANSWER would be worse than slow: the rule
//     would look up the id it asked about, find nothing, read the zero value
//     and wave through the very membership it was judging.
//   - AN ID THAT DOES NOT PARSE reaches no query and answers the zero row. The
//     domain already refuses it before asking, but a value the driver rejects
//     would turn a validation problem into a 500 — and "no row carries it" is
//     both safe and true. It is not memoised: there is nothing to remember.
//   - AN ID THE READ DID NOT ANSWER FOR lands as the zero row, which is
//     `Found: false` for all three row types — "no such row, or one this tenant
//     retired". Absent and archived collapse into one state deliberately; every
//     caller of these rows needs them to.
//   - THE MEMO IS PER REQUEST. Outside a request there is no context to
//     memoise on and every call is a fresh read, which is correct rather than
//     merely acceptable: a long-lived cache would answer with a bundle from an
//     arbitrary point in the past, and that bundle is what the escalation rules
//     are judging.
func ResolveRows[T any](
	ctx *configuration.AppContext,
	memoPrefix string,
	ids []domain.ID,
	read func(missing []domain.ID) map[string]T,
) map[domain.ID]T {
	var zero T
	out := make(map[domain.ID]T, len(ids))

	// Canonical id → every spelling this write asked about.
	wanted := make(map[string][]domain.ID, len(ids))
	for _, id := range ids {
		u, err := id.UUID()
		if err != nil {
			out[id] = zero
			continue
		}
		canonical := u.String()
		wanted[canonical] = append(wanted[canonical], id)
	}

	pending := make(map[string][]domain.ID, len(wanted))
	for canonical, spellings := range wanted {
		row, ok := MemoisedRow[T](ctx, memoPrefix, canonical)
		if !ok {
			pending[canonical] = spellings
			continue
		}
		for _, id := range spellings {
			out[id] = row
		}
	}
	if len(pending) == 0 {
		return out
	}

	missing := make([]domain.ID, 0, len(pending))
	for canonical := range pending {
		missing = append(missing, domain.NewID(canonical))
	}

	rows := read(missing)
	for canonical, spellings := range pending {
		row := rows[canonical]
		if ctx != nil {
			ctx.Set(memoPrefix+canonical, row)
		}
		for _, id := range spellings {
			out[id] = row
		}
	}
	return out
}

// MemoisedRow reads this request's answer for one canonical id, if it has one.
func MemoisedRow[T any](ctx *configuration.AppContext, memoPrefix string, canonical string) (T, bool) {
	var zero T
	if ctx == nil {
		return zero, false
	}
	cached, ok := ctx.Get(memoPrefix + canonical)
	if !ok {
		return zero, false
	}
	row, ok := cached.(T)
	return row, ok
}

// IDArgs is a requested set in the shape the criteria builder takes.
func IDArgs(ids []domain.ID) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// CanonicalIDOf is a stored row's id in the one spelling the resolvers key by.
// A stored id that is not a uuid answers "", which matches no request.
func CanonicalIDOf(id *domain.ID) string {
	if id == nil {
		return ""
	}
	u, err := id.UUID()
	if err != nil {
		return ""
	}
	return u.String()
}
