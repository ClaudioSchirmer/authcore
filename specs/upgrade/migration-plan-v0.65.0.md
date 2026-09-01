# Migration plan — omnicore v0.64.0 → v0.65.0

Status: APPROVED

Decision on the open item (§2): **`relational.clock: db`** in both profiles — authcore is
the IdP, its `deleted_at` stamps carry revocation semantics and its audit trail is read
and compared across replicas, so one fleet-wide clock is worth one round-trip per write
transaction.

Service: `authcore` · Build tags: `postgres` (engine from `relational.dialect`; neither
profile declares a `transport:` block, so no transport tag)

The bump is already applied — `go.mod`/`go.sum` are on v0.65.0 and
`go build -tags postgres ./...` is **green**. The rollback snapshot of the pre-bump
`go.mod`/`go.sum` is in `specs/upgrade/rollback/` (previous pin: `v0.64.0`).

One release sits in the range. It carries **one compile-visible break** (in a test file),
**one boot-blocking yaml item** that no compile surfaces, and three additive features that
are opportunities rather than obligations.

## Operational fallout — the five classes, checked

| Class | Present? | Evidence |
|---|---|---|
| (a) required DDL on the service's own tables | no | v0.65.0 adds no mandatory column; `StampedTimeField`/`StampedCounterField` are opt-in declarations |
| (b) demanded view rebuild | no | no `mongo:` block in either profile — the posture is pure relational source, there is nothing to rebuild |
| (c) framework embedded migration sequence grew | no | `infra/migration/**` is byte-identical between the two pins (30 `.sql` files, same set, same content) |
| (d) yaml key moved / renamed / **arrived mandatory** | **yes** | §2 — `relational.clock` |
| (e) shared gRPC proto contract changed | no | the project ships no `.proto` and no `.pb.go` |

---

## §1 — `core.Dialect` gained `UTCNowExpr()`; the test double no longer satisfies it

**The error, verbatim** (`go vet -tags postgres ./...`):

```
# github.com/ClaudioSchirmer/authcore/internal/infra
vet: internal/infra/authentication_reader_manual_test.go:73:22: cannot use testDialect{} (value of struct type testDialect) as core.Dialect value in variable declaration: testDialect does not implement core.Dialect (missing method UTCNowExpr)
```

Production code is unaffected — `go build -tags postgres ./...` is green. The only
implementer of `core.Dialect` in this repository is the `testDialect` stub that
`buildEffectivePermissionsStatement` is exercised against.

**How it worked at v0.64.0** — `infra/db/core/dialect.go`: the interface carried
`NowExpr() string` and nothing else clock-shaped. `NowExpr` is the framework's
control-plane bookkeeping stamp (outbox, failure ledgers), server-timezone by design.

**How it works at v0.65.0** — the same file adds one method:

```go
// UTCNowExpr renders the engine's SQL expression for the current instant in
// UTC, at the highest sub-second precision the engine offers. It is the
// source of the write operation's authoritative stamp under
// relational.clock: db (core.NowFrom), read ONCE per write transaction and
// then bound as an ordinary argument.
UTCNowExpr() string
```

It is deliberately not `NowExpr`: `NowExpr` is MySQL's bare `NOW()` (zero fractional
digits) and SQL Server's `CURRENT_TIMESTAMP` (server-local, ~3.33 ms rounding). Entity
timestamps can afford neither, because the unarchive cascade discriminates on exactly
that stamp. Postgres renders `NOW() AT TIME ZONE 'UTC'`
(`infra/db/engine/postgres/dialect.go`). `NowExpr()` is unchanged and keeps its callers.

**Proposed edit** — `internal/infra/authentication_reader_manual_test.go`, one method
added beside the existing `NowExpr`, following the file's own convention: the methods the
statement builder actually calls return a value, everything else panics so an unexpected
call is loud rather than silently plausible. `buildEffectivePermissionsStatement` builds a
read, never a write, so it never reaches for the write instant — this is a panic seat:

```go
func (testDialect) NowExpr() string    { return "NOW()" }
func (testDialect) UTCNowExpr() string { panic("unexpected UTCNowExpr") }
```

Mechanical; no judgment needed.

---

## §2 — `relational.clock` arrived MANDATORY, with no default — ANSWERED: `db`

**No compile error. The build is green and the service will not boot.** Neither
`microservice.dev.yaml` nor `microservice.prd.yaml` declares the key; both currently stop
at `dialect` + `dsn`.

**How it worked at v0.64.0** — the key did not exist. The write instant was
`time.Now().UTC()` in the writing process, unconditionally. (`yaml-reference.html` at
v0.64.0 has no `relational.clock`; its only "clock" is the JWT `leewaySeconds` skew.)

**How it works at v0.65.0** — `yaml-reference.html`, `relational:` block:

> `clock: db` — **MANDATORY — db | app. NO default, like dialect and dsn**: which clock a
> service's history is written against is an operator's declaration, and an absent value
> aborts boot. It governs the instant stamped on the managed timestamp columns —
> `created_at`, `updated_at` and the archive/unarchive `deleted_at` stamp.

The trade, stated the way the framework states it — including **why the instant is read
before the write**, without which `db` reads as a gratuitous round-trip:

- The instant is minted **once per operation and bound as an ordinary argument** — never a
  `NOW()` inside the DML — under **both** settings. One write is several statements (root,
  children, siblings, base cascade) that must all carry the same instant, and the value has
  to be known in Go **before `COMMIT`**: the outbox payload, the audit event, the lifecycle
  hooks and the response are all built from it. The unarchive cascade tells *this* archive's
  children from the ones already archived on their own by comparing exactly that stamp.
- **`db`** — one reading from Postgres per write transaction (`NOW() AT TIME ZONE 'UTC'`,
  microsecond precision, independent of the session time zone). Every replica shares one
  clock — the one the rows already live on. Cost: one extra round-trip per write TX.
- **`app`** — `time.Now().UTC()`, the v0.64.0 behavior. No round-trip; correct exactly as
  far as the fleet's clock discipline is. The app clock is a **per-POD** clock: two
  replicas drifting apart stamp rows out of order and nothing in the write path can notice.
- Neither setting makes `updated_at` an ordering token — that is `Revision`, the
  commit-order counter incremented under the row lock. `db` makes `updated_at` a
  trustworthy *display* timestamp, not a different kind of thing.

**Why this is an OPEN slot and not a mechanical edit:** the framework refused to pick a
default on purpose. A rename has one right answer; an arrival has a choice, and choosing
silently would be picking whose timestamps this service trusts.

**Bearing on `authcore` specifically** — the maintainer decides, but the facts:
`authcore` is the IdP. Its `deleted_at` stamps carry revocation semantics (a revoked
grant is an archived row), and its audit trail is the record of who was allowed what and
when. Both are read across replicas and compared against each other. One extra round-trip
per write TX is paid on writes only; reads are untouched.

**ANSWERED: `db`.**

**Proposed edit once answered** — the SAME value in **both** profiles, `prd` included
(a per-profile split would mean prd and dev write different histories):

- `microservice.dev.yaml`, inside `relational:`, after `dsn:` — `clock: db`
- `microservice.prd.yaml`, inside `relational:`, after `dsn:` — `clock: db`

Each with a short comment stating the trade, matching the surrounding files' commenting
density.

---

## §3 — Additive, no action required (opportunities)

None of these break anything; they are listed because they land directly on surfaces
`authcore` already uses, and ignoring them is a choice worth making knowingly.

- **`DirectWriter.Upsert`** — insert-or-update keyed on a declared conflict key
  (`write.OnConflict(goFields...)`, named per call by Go field name) instead of on the
  identity, in one statement, so two callers racing on the same key cannot both decide the
  row is missing. `authcore` merged its `DirectSchema` work in `5aeaf8d`/`ba80f43`; any
  place that currently does read-then-insert against a natural key is a candidate.
  Note two constraints: a schema declaring `DeletedAt` **must** also declare
  `write.UnarchiveOnConflict()` or `write.KeepArchiveStateOnConflict()` (an
  `INSERT … ON CONFLICT` has no `WHERE` for its conflict target, so an archived row still
  holds the unique key and still absorbs the write); and MySQL's `ON DUPLICATE KEY UPDATE`
  fires on any violated unique key, not the named one — irrelevant here, the service is
  Postgres-only.
- **`StampedTimeField`** — a timestamp column whose *when* is the domain's
  (`o.Stamp("Field")` from a rule, `Values{"Field": write.Stamp}` from a Direct write) and
  whose *value* is the framework's. `*time.Time`, never written from the struct; an
  unrequested stamp leaves the statement entirely. This is the seat
  `CreatedAt`/`UpdatedAt` do not cover — they date the ROW, this dates a FACT. Candidates
  in an IdP: last-authenticated, password-changed, consent-granted, token-revoked.
- **`StampedCounterField`** — a per-row `int64` counter the framework increments
  (`col = col + 1`, computed under the row lock, so racing increments cannot collapse).
  Per row, never per table — not a sequence. Deliberately absent from the outbox payload
  and from the write-back onto the entity. Candidate: failed-authentication attempt counts.
- **`Dialect.UTCNowExpr()`** — §1's method; also usable directly by hand-written infra
  that needs the engine's UTC instant at full precision.

Adopting any of these is a separate task (`/omnicore:evolve-entity` for a schema change
that needs a migration), not part of this upgrade.

---

## Behavioral items needing attention — not auto-fixable

- **`created_at` / `updated_at` / `deleted_at` change their source** the moment §2 is
  answered `db`: rows written after the bump carry the backend's instant, rows written
  before carry the writing pod's. Nothing migrates the old rows and nothing should — but a
  comparison that straddles the bump compares two clocks. Under `app` the source is
  unchanged and this item is void.
- **A green build is not a green boot.** §2 is invisible to `go build` and `go vet`; the
  only proof is a boot. After the edits are applied, run the service (`/omnicore:run`) and
  confirm both profiles start.

---

## Approval

§2 was answered `db` by the maintainer; §1 and §2 were then applied exactly as written
above, and nothing else was touched.
