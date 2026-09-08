// Tests for the collector both sign-in readers hand their grant walk to.
//
// WHAT THIS FILE PROTECTS is a guarantee that is invisible from either reader
// once it is delegated: a permission leaves this service ONCE, whichever path
// reached it and whichever kind of principal signed in. The readers prove their
// own walks against a database — that a retired role, a dropped membership and a
// revoked grant disappear from the answer. What they can no longer prove on their
// own is the collapse itself, because neither owns it any more.
//
// The three gates are asserted HERE as well as there, deliberately. There they
// are proven against real rows, which is the only place that means anything for a
// LEFT join's empty half; here they are proven exhaustively and cheaply, so a
// change that loosened one of them fails in the file that owns it rather than in
// a live suite somebody may not be able to run.

package infra

import (
	"testing"
	"time"
)

// The two pointer shapes add takes, at the call site's density rather than the
// caller's — a walk hands it columns, and a column is a pointer because a LEFT
// join can leave it NULL.
func ptr(s string) *string { return &s }

func stamp() *time.Time {
	t := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	return &t
}

// rendered is the shape the token carries, which is also the dedup key — so a
// test that reads the answer this way is reading exactly what a consumer would.
func rendered(s *permissionSet) []string {
	keys := s.sorted()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Resource+":"+k.Action)
	}
	return out
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// ── the collapse ────────────────────────────────────────────────────────────

// THE WHOLE REASON THIS TYPE EXISTS. A user reaches the same permission through a
// role held directly and through a role inherited from a group; a client reaches
// it through two roles of its own. Both arrive here as two rows, and both must
// leave as one entry.
func TestTheSamePairOfferedTwiceIsKeptOnce(t *testing.T) {
	var s permissionSet
	s.add(ptr("invoice"), ptr("read"), nil, nil)
	s.add(ptr("invoice"), ptr("read"), nil, nil)

	if got := rendered(&s); !equal(got, []string{"invoice:read"}) {
		t.Errorf("permissions = %v, want exactly [invoice:read]", got)
	}
}

// The collapse is by the PAIR and not by either half: two actions on one resource
// are two different permissions, and so are two resources sharing an action.
func TestPairsThatShareOneHalfAreNotTheSamePermission(t *testing.T) {
	var s permissionSet
	s.add(ptr("invoice"), ptr("read"), nil, nil)
	s.add(ptr("invoice"), ptr("delete"), nil, nil)
	s.add(ptr("report"), ptr("read"), nil, nil)

	want := []string{"invoice:delete", "invoice:read", "report:read"}
	if got := rendered(&s); !equal(got, want) {
		t.Errorf("permissions = %v, want %v", got, want)
	}
}

// ── the three gates, each on its own ────────────────────────────────────────

// A LEFT join's empty half: the role is real and held, and it confers nothing.
// The role still belongs in the token — that is the readers' business — but there
// is no permission here to carry.
func TestARowConferringNothingIsDropped(t *testing.T) {
	var s permissionSet
	s.add(nil, nil, nil, nil)
	s.add(ptr("invoice"), nil, nil, nil)
	s.add(nil, ptr("read"), nil, nil)

	if got := rendered(&s); len(got) != 0 {
		t.Errorf("permissions = %v, want none — no row named both halves", got)
	}
}

func TestARevokedGrantIsDropped(t *testing.T) {
	var s permissionSet
	s.add(ptr("invoice"), ptr("read"), stamp(), nil)

	if got := rendered(&s); len(got) != 0 {
		t.Errorf("permissions = %v, want none — the role's grant of it was revoked", got)
	}
}

func TestARetiredCatalogEntryIsDropped(t *testing.T) {
	var s permissionSet
	s.add(ptr("invoice"), ptr("read"), nil, stamp())

	if got := rendered(&s); len(got) != 0 {
		t.Errorf("permissions = %v, want none — the catalog entry itself was retired", got)
	}
}

// A GATE DROPS THE ROW, NEVER THE PAIR. The same permission reached by a revoked
// grant on one role and a live grant on another is HELD: the principal has it,
// through the second role. A gate that remembered the refusal would revoke a
// grant nobody revoked.
func TestAPairRefusedOnOneRowSurvivesOnAnother(t *testing.T) {
	var s permissionSet
	s.add(ptr("invoice"), ptr("read"), stamp(), nil)
	s.add(ptr("invoice"), ptr("read"), nil, nil)

	if got := rendered(&s); !equal(got, []string{"invoice:read"}) {
		t.Errorf("permissions = %v, want [invoice:read] — a live grant of the same "+
			"pair reached it through another role", got)
	}
}

// ── the order, and the empty answer ─────────────────────────────────────────

// Resource first, then action — the order two tokens minted from the same grants
// have to agree on, or a diff between them stops being readable.
func TestTheAnswerIsSortedByResourceThenAction(t *testing.T) {
	var s permissionSet
	s.add(ptr("user"), ptr("read"), nil, nil)
	s.add(ptr("invoice"), ptr("read"), nil, nil)
	s.add(ptr("user"), ptr("archive"), nil, nil)
	s.add(ptr("invoice"), ptr("delete"), nil, nil)

	want := []string{"invoice:delete", "invoice:read", "user:archive", "user:read"}
	if got := rendered(&s); !equal(got, want) {
		t.Errorf("permissions = %v, want %v", got, want)
	}
}

// The zero value answers without being built, and a walk that reached nothing
// pays no allocation. NIL rather than an empty slice is what both readers
// answered before this type existed; RenderPermissions is what turns it into the
// present-and-empty claim a consumer reads.
func TestAWalkThatReachedNothingAnswersNil(t *testing.T) {
	var s permissionSet

	if got := s.sorted(); got != nil {
		t.Errorf("permissions = %v, want nil", got)
	}
}
