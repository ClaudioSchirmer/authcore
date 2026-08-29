// Tests for the batched row resolution that needs no database.
//
// resolveRows is where every property of a BATCH lives — how the requested set
// is deduplicated, what an id that does not parse costs, which spelling the
// memo is keyed by, and what an id the read did not answer for means. The read
// itself is a closure, so all of it is exercised here with a fake one: what is
// asserted is the resolution, not the SQL.

package infra

import (
	"testing"

	"github.com/ClaudioSchirmer/omnicore/application/configuration"
	"github.com/ClaudioSchirmer/omnicore/domain"
)

// resolvedTestRow is a row type standing in for the three real ones. Its zero value is
// "not found", which is the property every caller of a resolved row relies on.
type resolvedTestRow struct {
	found bool
	label string
}

const (
	lowerCaseID = "0198f3e0-9c25-7a1f-b73d-5e08c4a29f61"
	upperCaseID = "0198F3E0-9C25-7A1F-B73D-5E08C4A29F61"
)

// THE SPELLING THE CALLER USED IS THE SPELLING THE ANSWER IS KEYED BY, and the
// row is read ONCE however many spellings of it the write carries.
//
// uuid.Parse accepts an upper-case id and the store renders it lower case, so a
// resolver keyed by the raw string would read one row twice — and, far worse, a
// resolver ANSWERING under the canonical spelling would hand the rule a map it
// cannot look anything up in: the rule would find nothing, read the zero value
// and wave through the very entry it was judging.
func TestOneRowAnswersEverySpellingTheWriteAsksWith(t *testing.T) {
	asked := 0
	requested := []domain.ID{domain.NewID(lowerCaseID), domain.NewID(upperCaseID)}

	rows := resolveRows(nil, "test.row:", requested, func(missing []domain.ID) map[string]resolvedTestRow {
		asked++
		if len(missing) != 1 {
			t.Errorf("the read was asked for %d ids, want the one row both spellings name", len(missing))
		}
		return map[string]resolvedTestRow{lowerCaseID: {found: true, label: "resolved"}}
	})

	if asked != 1 {
		t.Errorf("the store was read %d times for one row, want exactly 1", asked)
	}
	for _, id := range requested {
		if !rows[id].found {
			t.Errorf("the answer carried nothing for %q, the spelling the caller passed", id.String())
		}
	}
}

// AN UNUSABLE ID REACHES NO QUERY and answers the zero row. The domain refuses
// it before asking, but a value the driver would reject must never bind into a
// criterion against a UUID column: that turns a validation problem into a 500.
func TestAnUnusableIDIsAnsweredWithoutAReadAtAll(t *testing.T) {
	read := func([]domain.ID) map[string]resolvedTestRow {
		t.Fatal("an unusable id reached the store")
		return nil
	}

	unusable := []domain.ID{domain.NewID(""), domain.NewID("tatu")}
	rows := resolveRows(nil, "test.row:", unusable, read)

	for _, id := range unusable {
		if rows[id].found {
			t.Errorf("%q resolved to a row", id.String())
		}
	}
}

// AN ID THE READ DID NOT ANSWER FOR is absent from its result and lands as the
// zero row — "no such row, or one this tenant retired". The two collapse
// deliberately: every caller of these rows needs them to.
func TestAnIDTheReadDidNotAnswerForIsNotFound(t *testing.T) {
	missing := domain.NewID("0198f3e0-1a44-7bb2-9c31-77c0d5e1b904")

	rows := resolveRows(nil, "test.row:", []domain.ID{missing}, func([]domain.ID) map[string]resolvedTestRow {
		return map[string]resolvedTestRow{}
	})

	if _, ok := rows[missing]; !ok {
		t.Fatal("the answer left the id out entirely; every requested id must carry a verdict")
	}
	if rows[missing].found {
		t.Error("an id the store did not answer for was reported as found")
	}
}

// THE MEMO IS WHAT MAKES THREE FACTS ONE QUERY. The second question about a row
// this request already resolved must not read it again — and it is keyed by the
// canonical id, so a second fact asking with a different spelling still hits.
func TestASecondQuestionAboutTheSameRowReadsNothing(t *testing.T) {
	// The framework's own constructor, not a zero value: a bare AppContext
	// carries no metadata map, and the memo writes into it.
	ctx := configuration.NewAppContextWithRandomID(configuration.LangENG)
	reads := 0
	read := func([]domain.ID) map[string]resolvedTestRow {
		reads++
		return map[string]resolvedTestRow{lowerCaseID: {found: true, label: "resolved"}}
	}

	resolveRows(ctx, "test.row:", []domain.ID{domain.NewID(lowerCaseID)}, read)
	second := resolveRows(ctx, "test.row:", []domain.ID{domain.NewID(upperCaseID)}, read)

	if reads != 1 {
		t.Errorf("the row was read %d times across two questions, want exactly 1", reads)
	}
	if !second[domain.NewID(upperCaseID)].found {
		t.Error("the memoised row did not answer the second question")
	}
}

// WITHOUT A REQUEST THERE IS NO MEMO, and every call is a fresh read. That is
// correct rather than merely acceptable: a cache outliving the request would
// answer with a bundle from an arbitrary point in the past, and that bundle is
// what the escalation rules judge.
func TestOutsideARequestNothingIsRemembered(t *testing.T) {
	reads := 0
	read := func([]domain.ID) map[string]resolvedTestRow {
		reads++
		return map[string]resolvedTestRow{lowerCaseID: {found: true}}
	}

	resolveRows(nil, "test.row:", []domain.ID{domain.NewID(lowerCaseID)}, read)
	resolveRows(nil, "test.row:", []domain.ID{domain.NewID(lowerCaseID)}, read)

	if reads != 2 {
		t.Errorf("the row was read %d times outside a request, want one read per call", reads)
	}
}

// The two helpers the resolvers hand the store, asserted where they are cheap
// to assert: the criteria argument list, and the stored row's key.
func TestTheStoredRowIsKeyedByItsCanonicalID(t *testing.T) {
	id := domain.NewID(upperCaseID)
	if got := canonicalIDOf(&id); got != lowerCaseID {
		t.Errorf("canonicalIDOf(%q) = %q, want the canonical spelling", upperCaseID, got)
	}
	if got := canonicalIDOf(nil); got != "" {
		t.Errorf("canonicalIDOf(nil) = %q, want the empty key no request matches", got)
	}
	unusable := domain.NewID("tatu")
	if got := canonicalIDOf(&unusable); got != "" {
		t.Errorf("canonicalIDOf(%q) = %q, want the empty key no request matches", "tatu", got)
	}
	if got := idArgs([]domain.ID{id}); len(got) != 1 || got[0] != any(id) {
		t.Errorf("idArgs did not carry the id through: %v", got)
	}
}
