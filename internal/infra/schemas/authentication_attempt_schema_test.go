// The drift guard between the attempt rollup's schema and the table it maps.
//
// This schema is the ONLY description of authentication_attempts the Go side
// has — there is no aggregate declaring it a second time — so nothing else would
// catch a column renamed on one side and not the other. The failure mode is
// silent and expensive: a write naming a column that no longer exists fails at
// the driver on the sign-in path, and a read of the wrong column answers "not
// locked" to every attempt.
//
// So the DDL is read and compared. A migration that renames a column, or a schema
// that stops declaring one, fails here — naming the column — instead of at three
// in the morning.
//
// WHAT IS STAMPED IS ASSERTED, NOT ASSUMED. The three counters are what make the
// rollup safe under concurrency (`col = col + 1`, evaluated by the server under
// the row's lock) and the two instants are what keep the row on the database's
// clock. If one of them were declared an ordinary Field, every write would still
// compile and still run — two simultaneous failures would simply count as one,
// which is a lockout quietly running short.

package schemas

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
)

const attemptMigration = "../../../migrations/postgres/0007_authentication_attempts_manual.up.sql"

// attemptDDL is the CREATE TABLE body, with the comment lines dropped so a column
// merely MENTIONED in prose cannot pass for one that is declared.
func attemptDDL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(attemptMigration)
	if err != nil {
		t.Fatalf("reading %s: %v", attemptMigration, err)
	}
	var body strings.Builder
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	return body.String()
}

// declaresColumn reports whether the DDL declares `"col"` as a column of the
// table — the quoted name at the start of a line, which is how every column in
// this migration is written.
func declaresColumn(ddl, col string) bool {
	return regexp.MustCompile(`(?m)^\s*"` + regexp.QuoteMeta(col) + `"\s`).MatchString(ddl)
}

// EVERY COLUMN THE SCHEMA NAMES EXISTS IN THE TABLE. Resolve is the surface every
// read and write consults, so asking it here asks exactly what production asks.
func TestAuthenticationAttemptSchema_EveryColumnExistsInTheMigration(t *testing.T) {
	schema := AuthenticationAttemptSchema()
	ddl := attemptDDL(t)

	if !declaresColumn(ddl, schema.IDColumn()) {
		t.Errorf("the schema's primary key %q is not a column of the table", schema.IDColumn())
	}
	for _, goField := range []string{
		"Identity", "IdentityKind", "Outcome",
		"TotalCount", "CurrentCount", "TotalBlocked",
		"WindowStartedAt", "LastAt", "LastIP", "IdentityExisted",
	} {
		resolved, ok := schema.Resolve(goField)
		if !ok {
			t.Errorf("%q does not resolve on the schema — the store names it in a write", goField)
			continue
		}
		if !declaresColumn(ddl, resolved.Column) {
			t.Errorf("%q resolves to column %q, which the 0007 migration does not declare",
				goField, resolved.Column)
		}
	}
}

// THE CONCURRENCY-SAFE COLUMNS, asserted as such. A counter declared as an
// ordinary Field would take the caller's value instead of the server's
// increment, and two failures landing together would count as one.
func TestAuthenticationAttemptSchema_TheThreeCountersAreStampedCounters(t *testing.T) {
	schema := AuthenticationAttemptSchema()

	for _, goField := range []string{"TotalCount", "CurrentCount", "TotalBlocked"} {
		if !schema.IsStampedField(goField) {
			t.Errorf("%q is not stamped — its value would be the caller's, and two concurrent "+
				"attempts could both write back the same number", goField)
			continue
		}
		if !schema.IsStampedCounter(goField) {
			t.Errorf("%q is stamped as an INSTANT rather than a counter — filling it would date "+
				"the row instead of counting the attempt", goField)
		}
	}
}

// THE TWO INSTANTS, likewise. They carry the write operation's own moment, read
// from the database under `relational.clock: db` — which is what keeps a lockout
// window from being measured against a drifted pod clock.
func TestAuthenticationAttemptSchema_TheTwoInstantsAreStampedTimes(t *testing.T) {
	schema := AuthenticationAttemptSchema()

	for _, goField := range []string{"WindowStartedAt", "LastAt"} {
		if !schema.IsStampedField(goField) {
			t.Errorf("%q is not stamped — the row would be dated by whichever pod served the request", goField)
			continue
		}
		if schema.IsStampedCounter(goField) {
			t.Errorf("%q is stamped as a COUNTER — filling it would increment a timestamp", goField)
		}
	}
}

// THE CONFLICT KEY CANNOT BE STAMPED. An upsert writes its key once, on the row
// it creates, and the framework refuses a stamped column there — so a key field
// that drifted into the stamped family would break every write on this table.
func TestAuthenticationAttemptSchema_TheNaturalKeyIsOrdinary(t *testing.T) {
	schema := AuthenticationAttemptSchema()

	for _, goField := range []string{"Identity", "IdentityKind", "Outcome"} {
		if schema.IsStampedField(goField) {
			t.Errorf("%q is stamped, and it is part of the upsert's conflict key — "+
				"the framework refuses a stamped slot on a key, so every write here would fail", goField)
		}
	}
}

// THE NATURAL KEY THE STORE CONFLICTS ON IS THE ONE THE TABLE ENFORCES. If the
// UNIQUE constraint named a different triple, the upsert would insert a new row
// per attempt — the unbounded table the rollup exists to prevent.
func TestAuthenticationAttemptSchema_TheUniqueConstraintMatchesTheConflictKey(t *testing.T) {
	schema := AuthenticationAttemptSchema()
	ddl := attemptDDL(t)

	var cols []string
	for _, goField := range []string{"Identity", "IdentityKind", "Outcome"} {
		resolved, ok := schema.Resolve(goField)
		if !ok {
			t.Fatalf("%q does not resolve", goField)
		}
		cols = append(cols, `"`+resolved.Column+`"`)
	}
	want := "UNIQUE (" + strings.Join(cols, ", ") + ")"
	if !strings.Contains(ddl, want) {
		t.Errorf("the table declares no %s — an upsert conflicting on a key the table does not "+
			"enforce inserts a new row per attempt", want)
	}
}

// A Direct anchor, and nothing else. read.NewDirectRepository refuses any other
// kind at construction, so this is the boot contract stated where it is decided.
func TestAuthenticationAttemptSchema_IsADirectAnchor(t *testing.T) {
	schema := AuthenticationAttemptSchema()

	if !schema.IsDirect() {
		t.Error("the schema is not Direct — the repository would refuse it at construction")
	}
	if schema.Table() != "authentication_attempts" {
		t.Errorf("table = %q", schema.Table())
	}
	// NO archive column, deliberately: declaring one would silently gate every
	// read on a column the migration never created.
	if col, has := schema.ArchivedAtColumn(); has {
		t.Errorf("the schema declares the archive column %q, which this table does not have — "+
			"every read would be gated on it", col)
	}
}

// A compile-time reminder that the row type is what the schema is anchored to.
var _ = core.NewDirectSchema[AuthenticationAttempt]
