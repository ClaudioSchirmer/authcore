# Capability plan — authentication-attempt-counters

- **Status:** APPLIED (2026-08-27), with **one defect shipped and fixed the same day** — the
  re-anchor predicate missed the NULL-anchor state, making every identity that had ever signed
  in successfully immune to the lockout. Found by the maintainer in manual use, not by this
  plan's verification. See §3.2 for the fix and §7 for why the proof missed it.
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.61.0` (that pin's docs are the
  authority for every line below)

## §1 The request (restated)

> Eu quero mudar a ideia de authentication_attempts, prefiro que ela segure um contador total +
> um contador zerável para entender o lock. Uma linha para success e uma linha para failure e o
> locked é teu contador, não mais uma linha na tabela. Aí da para fazer um select direto na
> linha certa para otimizar a performance e diminuir a sobrecarga de dados. Todo login success
> ou fail vc joga no log tbm via event de log da aplication com o publish. Da uma analisada
> nisso. Estou muito preocupado com a performance dessa tabela crescendo. Duas linha por usuário
> é o ideal máximo. Da uma analisada e me monta o plano para eu entender e aprovar a mudança. Aí
> minha sugestão é alterar o script 0007 direto e vc roda manualmente um delete table + o script
> novo para conformidade da minha base de dados.

When this is done, `authentication_attempts` stops being an append-only journal and becomes a
**rollup table with a hard ceiling of two rows per (identity, kind)**: one `failure` row and
one `success` row. The lockout stops being a windowed `COUNT` over N rows and becomes a
**resettable counter read by primary key** — one index probe, one row, no scan, and a table
whose size is bounded by the number of distinct identities ever tried rather than by traffic.
Each row also carries a **lifetime total** that is never reset, so "how many times has this
identity ever failed" survives every window reset and every successful sign-in.

The per-attempt narrative that the table used to hold — every attempt, its IP, its timestamp —
moves to the **service's always-on structured log stream**, published through the framework's
`events.Publisher` port on every sign-in outcome. The forensic questions are still answerable;
they are answered by the log pipeline (collector → Elasticsearch/Loki → Kibana) instead of by
SQL. **That relocation is a functional change, not a refactor — see §3.1, which is the one
decision this plan will not take on its own.**

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| Publishing a fact to the service's log stream from application code | `domain-events.html` — the `events.Publisher` port (`Publish(ctx persistence.RequestContext, event domain.Event) error`), the `domain.DomainEvent` struct (`Type`/`Class`/`Msg`/`Vals`/`Reason`), the four `EventType` severities and their slog levels, and `events.NewSlogPublisher(logger)` as the default transport | offered at pin — verified in source: `infra/events/publish.go` (port + `SlogPublisher`), `domain/event.go` (`Event` interface + `DomainEvent`), `bootstrap/bootstrap.go:569` (`eng.WithEventPublisher(events.NewSlogPublisher(logger))`) |
| The log channel the event lands on | `logs.html` — the always-on structured-JSON stdout channel and the external observability stack it feeds | offered at pin |
| Hand-written SQL over the neutral engine seam | `architecture.html` (invariant 9: `DB.Querier()` for custom reads, statement execution via `core.Exec`), `infra/db/core/read.go` (`Querier`, `ExecQuerier`, `core.Exec`), `infra/db/core/dialect.go` (`Placeholder`/`QuoteIdent`/`EncodeArg`/`NowExpr`/`ApplyLimit`) | offered at pin |
| **Dialect-neutral upsert with a counter increment** | `infra/db/core/dialect.go` — `Dialect.BuildUpsert(table, cols, conflictCols, []UpsertSet)` and the three `UpsertSetMode`s: `UpsertSetNew` (the proposed value), `UpsertSetExpr` (a verbatim expression that **must not reference a target-table column**), `UpsertSetBump` (`existing + 1`) | offered at pin — implemented on **all five** engines (`postgres`/`sqlite` → `ON CONFLICT … DO UPDATE`, `mysql` → `… AS new ON DUPLICATE KEY UPDATE`, `sqlserver` → `MERGE … WITH (HOLDLOCK)`, `oracle` → `MERGE INTO … FROM dual`). Reference consumer: `infra/integration/failures.go:107` |
| Editing an already-applied migration + manual conformance | `migrations.html` — `omnicore_migrations` stores **only** `(version BIGINT PRIMARY KEY, dirty BOOLEAN)`, one row, **no checksum of the file** | offered at pin — verified live: `SELECT * FROM omnicore_migrations` returns `version=8, dirty=f` |
| Notification layering for a handler-raised rejection | `status-mapping.html` + `shared/notification-bases.md` | offered at pin — `N/A` here: no new notification, see §5 |

**Routing outcome: offered at pin.** No upgrade needed, no infra to enable — the default
`SlogPublisher` is wired by `bootstrap.Run` unconditionally, needs no broker, no Mongo and no
yaml. A domain event never leaves the process; its transport is this service's own log stream.

**One routing caveat, stated rather than glossed.** `domain-events.html` frames a domain event
as *a fact the entity authors*, registered via `BaseEntity.RegisterEvent` and published by the
write engine post-commit. A sign-in is not an entity write and has no such seat. Publishing
**directly** through the port is nonetheless a supported use at this pin, and not an
improvisation: the doc calls `events.Publisher` "a port: implement it to send domain events
somewhere other than the log stream", and `infra/events/event.go` documents `EntityType` as
`omitempty` precisely because "a domain event need not be bound to one specific entity class
(system-level events, scheduled jobs, etc. emit with an empty ClassName)". A sign-in outcome is
exactly that system-level event.

## §3 Integration semantics [high-risk — propose + CONFIRM]

### §3.1 The forensic record leaves SQL. This is a REMOVAL — DECIDED.

Migration `0007`'s own header states the table "IS TWO THINGS AT ONCE": the lockout **and** the
forensic record, and that the forensic half "is the product, not a by-product". A two-row
rollup keeps the first and **cannot** keep the second. Concretely, these stop being answerable
in SQL:

| Question the append-only table answers today | After the change |
|---|---|
| "What has **this IP** been doing?" (`authentication_attempts_ip_time_idx`) | **gone from SQL.** A rollup row keyed by identity holds only the *last* IP. One IP walking a hundred identities is invisible to the database. |
| "Show me the burst at 04:00" — the per-attempt timeline | **gone from SQL.** Only `first`/`last` timestamps survive per row. |
| "Which identities were tried, in what order, from where" | **gone from SQL.** |
| "How many failures has this identity ever had" | **kept** — that is `total_failures`, and it is now exact and free rather than a `COUNT`. |
| "Is this identity locked, and until when" | **kept, and cheaper** — one PK probe instead of a windowed scan. |
| "Is a locked identity a real account or noise" | **kept** — `identity_existed` rides the failure row. |

The three lost questions move to the log stream, where every attempt is published as one
`"event"` record carrying identity, kind, outcome, IP and the existence flag. Kibana/Loki
answer them at least as well as SQL did — that is what a SIEM is for. But the properties are
different and the difference is not cosmetic:

- **Retention becomes the log pipeline's policy**, not the maintainer's DB decision. `0007`'s
  header explicitly reserves retention as "the maintainer's policy call"; after this change the
  collector's retention window silently becomes that policy.
- **Delivery becomes best-effort.** See §3.4.
- **Identities in clear now flow off-box** into the observability stack — including identities
  of people who never registered. Today they sit in one table in one database. `0007` already
  flagged that accumulation as "the other reason retention is a decision somebody has to take";
  the log pipeline typically has wider read access than the database does.

**DECIDED (maintainer, 2026-08-27): REMOVE.** The log stream is the sole forensic home. The
rollup is the only table; per-IP and timeline questions are answered by the observability
stack. The consequence that must not be forgotten is in §7: until the collector actually
ingests and retains these records, the forensic record has left SQL without arriving anywhere
queryable.

### §3.2 The lockout algorithm — sliding count becomes a fixed window with a reset

**Seam:** unchanged. Still `AttemptRecorder` (application-owned port) implemented by
`internal/infra.AuthenticationAttemptStore`, still called synchronously in
`IssueTokenHandler.Handle`, still the same call order (lock probe first, record last).

**The row shape.** One table, natural key `(identity, identity_kind, outcome)`, `outcome`
constrained to exactly `'failure'` and `'success'` — the ceiling of two rows per identity is a
**database constraint**, not a convention.

| Column | Failure row | Success row |
|---|---|---|
| `total_count` | lifetime failures, **never reset** | lifetime successful sign-ins |
| `current_count` | failures inside the live window — **the resettable counter**; this IS the lock | unused — always `0` |
| `window_started_at` | when `current_count` last went 0 → 1; the window's anchor | unused — always `NULL` |
| `last_at` | most recent failure | **most recent successful sign-in** — a genuinely useful field the old design could not offer cheaply |
| `last_ip` | origin of the most recent failure | origin of the most recent success |
| `identity_existed` | last established answer (`NULL` when nothing established it) | always `true` — a credential cannot verify against an absent account |

**Reading the lock** — one statement, one row, by primary key:
`current_count >= LockoutThreshold AND window_started_at > now - LockoutWindow`.
The expiry stays **derived**, never stored: `window_started_at + LockoutWindow`. Nothing to
clear, nothing that can drift — the same property the append-only design had.

**Recording a failure** — two statements, both rendered through the framework's dialect seam,
**no engine-specific SQL written by hand** (see §3.6.4):

1. **The re-anchor**, a plain `UPDATE` by the natural key that opens a new burst whenever
   there is no live one, setting `current_count = 0` and `window_started_at = NowExpr()`. In
   the case that matters — an attack in progress, window still live — it matches **zero rows**
   and costs one index probe.

   **"No live burst" is TWO states, and they need two predicates:**
   `AND (window_started_at IS NULL OR window_started_at <= <windowStart>)`. The anchor is NULL
   before the first failure *and* after a successful sign-in, and `NULL <= x` evaluates to NULL
   rather than TRUE — so a bare comparison skips exactly the rows that most need re-anchoring.
   Because statement 2 never writes this column on conflict, this statement is its only other
   writer: miss the NULL state and the counter climbs forever against a NULL anchor, no expiry
   can be derived, and every identity that has ever signed in becomes permanently unlockable.
   **This was shipped wrong and found in manual use — see §7.**
2. **The upsert**, built by `Dialect.BuildUpsert` with `conflictCols = (identity,
   identity_kind, outcome)`: `total_count` and `current_count` both `UpsertSetBump`, `last_at`
   and `last_ip` `UpsertSetNew`. `window_started_at` is deliberately **absent from the update
   set** — it is written by the INSERT branch on the first failure and by statement 1 on a
   re-anchor, so on a live window it keeps pointing at the burst's first failure, which is
   exactly what the expiry is derived from. That absence is also what makes statement 1's
   predicate load-bearing rather than an optimisation.

Splitting it this way is what removes the conditional. `UpsertSetExpr` forbids referencing a
target-table column (each engine names the existing row differently — `EXCLUDED` / `new` /
`target`), so a `CASE WHEN window_started_at > … THEN current_count + 1 ELSE 1 END` cannot be
expressed portably. Hoisting the reset into its own statement makes every remaining assignment
one the seam already speaks.

`identity_existed` rides `UpsertSetNew` **only when the caller has a non-nil answer**; when the
lookup established nothing, the column is simply left out of the update set so the previously
established value survives rather than being overwritten with `NULL`. Both statement variants
are assembled once at construction, keeping the store's existing rule that whatever is wrong
with a statement is wrong at boot.

**Both increments are computed in SQL, never read-modify-write in Go**, so concurrent failed
attempts serialize on the row lock and none is lost. That answers the concern `0007` raised
against a counter column — it feared a *revision guard* producing 409s; neither statement has a
revision guard and neither can conflict.

**The cost, stated plainly:** the failure path goes from one statement to two. The path that
actually decides the DoS economics — the lock probe, which every request past the threshold
short-circuits on — goes the other way, from a windowed scan of N rows to a single read by
primary key.

**Recording a success** — two statements, in this order: first reset the failure row
(`current_count = 0`), then upsert the success row. The reset is the load-bearing half and goes
first, so a failure between them leaves the user *unlocked*, never spuriously locked.

**What changes behaviourally.** The window goes from *sliding over the N most recent failures*
to *fixed, anchored at the first failure of the burst*. For the shape that matters — 5 failures
in quick succession — the two are identical: locked until `first_failure + 15min`. They diverge
for failures spread thinly across a window boundary, where the fixed window is marginally more
permissive. Both are standard; the fixed window is what makes the single-row read possible.
Attempts made *during* a lock still do not extend it: the handler returns before
`RecordFailure` is reached, so neither counter moves. That property is preserved exactly.

**What is preserved without qualification:** keying by the *attempted* identity (so an unknown
address locks exactly as a real one does — no existence oracle in the message or the response
time); the lock surviving a restart; auto-release with no scheduled job; the attempted password
never having a column it could enter.

### §3.2b The growth ceiling, stated exactly

The request's target — "duas linhas por usuário é o ideal máximo" — is met, with one wording
correction that matters operationally: it is **two rows per *identity ever tried*, not per
*registered user***. That follows directly from the anti-oracle property that must be kept:
the table is keyed by the ATTEMPTED identity, so an address nobody ever registered still gets
a row, exactly as a real one does. Removing that would reopen the existence oracle `0007` was
built to close.

So the growth curve changes from **O(attempts)** to **O(distinct identities tried)**. For
ordinary traffic that is the difference between a table that grows forever and one that
plateaus at roughly the user count — precisely the win being asked for. Under a
credential-stuffing run against a leaked list of a million addresses it is still a million
rows; it is simply a million instead of the tens of millions the append-only design would have
taken. The rollup is strictly better on every axis and unbounded on none that the old design
bounded, but it is **not** bounded by the number of real accounts, and sizing should assume
"distinct identities an attacker chooses to try", not "our users".

A sweep is therefore still worth having eventually — a row whose `last_at` is old and whose
`current_count` is 0 carries nothing the log stream does not. It is **out of scope here** and
noted so it is a decision rather than an omission.

### §3.3 The log publish — who calls it, and with what

**Seam — DECIDED: the application handler publishes, not the infra store.** The store stays a
pure counter with one job. The handler already holds the whole narrative — which branch
refused, whether the account existed, what the lock verdict was — and the event carries facts
the store's parameters never see. It also matches the request literally: *"event de log da
**aplication**"*.
*Alternative considered and rejected:* folding the publish into `RecordFailure`/`RecordSuccess`
would give one call site, but makes a counter store responsible for narration and caps the
event at what the store happens to receive.

**No adapter, no invented type.** The application declares a one-method port beside
`AttemptRecorder`, spelled in types it may already import (`application/persistence` and
`domain` are both legal above infra):

- port method: `Publish(ctx persistence.RequestContext, event domain.Event) error`
- `*configuration.AppContext` already satisfies `persistence.RequestContext` (verified:
  `ID()`, `ActorSubject()`, `ActorIssuer()`, `ActorClaims()`, plus `context.Context`)
- `*events.SlogPublisher` satisfies the port **structurally** — bootstrap passes
  `events.NewSlogPublisher(d.Logger)` straight in. No wrapper type is written.

**Sync or async:** synchronous, inline in the handler. `SlogPublisher.Publish` is a single
`LogAttrs` call to the stdout handler — no IO worth deferring, and ordering with the counter
write stays obvious.

**What each event carries** (`Vals`): `identity`, `identityKind`, `outcome`, `ip`,
`identityExisted`, and on a refusal the counter state (`currentFailures`, and `lockedUntil` on
the locked branch). `Class: "Authentication"` so `entityType` is filterable.
**Never**: the attempted password, in any form — no clear, no hash, no length. Same rule as the
table, and there is no field it could occupy.

**Severity — DECIDED:** failure → `domain.EventWarning` (slog `Warn`) · refused-while-locked →
`domain.EventWarning` · success → `domain.EventLog` (slog `Info`). A failed sign-in is the
thing an operator alerts on; a success is a routine record.

### §3.4 Failure policy — and the one place it is genuinely load-bearing

Two different failures, deliberately answered differently:

- **The counter write fails** → **propagates → 500**, unchanged from today. `0007`'s reasoning
  stands untouched: "a brute-force protection that silently stops counting is worse than one
  that was never built — nobody would know." An authentication that cannot reach its store must
  not answer.
- **The publish fails** → **DECIDED: swallowed + one `slog.Warn`**, mirroring the framework's
  own posture (`domain-events.html`: "a publisher error is logged as `slog.Warn`
  `event.publish.error` and swallowed… do not put anything you cannot afford to lose behind
  this port"). The lockout — the load-bearing half — is in SQL and is not behind this port.
  *This is the honest cost of §3.1:* once the forensic record's only home is the log stream, it
  sits behind a best-effort port. `SlogPublisher` writes to stdout and realistically does not
  fail, but the guarantee is weaker than an in-TX `INSERT`, and that should be an accepted
  trade rather than a discovered one.
  Propagating instead (→ 500) was considered and rejected: it would let a full log buffer refuse
  valid sign-ins.

### §3.5 Idempotency / replay

`N/A` — no receiver, no broker, no redelivery. A retried HTTP sign-in is a new attempt and is
correctly counted as one.

### §3.6 The smaller decisions, each on its own axis

Listed separately because each has its own criterion; a packaged answer hides which one is
being decided.

1. **The `locked` outcome disappears as a row — DECIDED: `total_blocked` on the failure row.**
   A lifetime counter that never resets and never touches `window_started_at`, so an attempt
   made during a lock still cannot extend it — the exact property the `locked` row existed to
   preserve. It costs one write on the locked path, which is what that path costs today, so it
   is not a regression. `RecordLocked` therefore SURVIVES, as a counter-only method.
2. **The key — DECIDED: simple PK, natural key as a UNIQUE constraint.** This project works
   with simple primary keys only; a composite PK is not an option. So `id UUID PRIMARY KEY`
   **stays** (nothing is removed here after all) and the two-row ceiling is enforced by
   `CONSTRAINT authentication_attempts_natural_key UNIQUE (identity, identity_kind, outcome)`.
   It is still a database constraint rather than a convention — the table physically cannot
   hold a third row for one identity+kind.

   This is exactly the framework's own shape for the same problem: `omnicore_integration_failures`
   declares `id UUID PRIMARY KEY` plus `CONSTRAINT omnicore_integration_failures_natural_key
   UNIQUE (consumer_group, source_key, event_key, event_id)`, and upserts against the natural
   key. `BuildUpsert`'s `conflictCols` takes that unique constraint, not the PK — Postgres and
   SQLite infer the unique index, MySQL triggers on any unique key, and the MERGE dialects match
   on the `ON` clause. `id` rides in the INSERT columns and is **never** in the update set, so a
   fresh id is minted on the insert branch and the existing row keeps its own on conflict
   (`infra/integration/failures.go:67,107`). The id stays `domain.NewRandomID()` as today.

3. **The success row — DECIDED: it carries the lifetime success count and the last success
   date, and nothing else.** `total_count` = successes ever, `last_at` = when the last
   successful sign-in happened (plus `last_ip`, free from the same write). The lockout's
   machinery — `current_count`, `window_started_at`, `total_blocked` — is failure-only and
   stays `0` / `NULL` / `0` on the success row, stated in the column comments so the emptiness
   reads as declared rather than forgotten. *Rejected alternative:* making `current_count` mean
   "successes since the last failure" would force the **failure** path to write the success row
   too, doubling the hot path's cost to answer a question nobody asked.
4. **Dialect neutrality of the store — NO LONGER A DECISION; an earlier claim in this plan was
   wrong.** A first pass reported that `core.Dialect` exposes no upsert primitive. It does:
   `BuildUpsert(table, cols, conflictCols, []UpsertSet)`, implemented on all five engines, with
   a `UpsertSetBump` mode whose entire reason for existing is the counter increment this design
   needs ("it exists because no verbatim expression can spell 'the existing row's column'
   portably"). **The store therefore stays fully dialect-neutral and nothing is pinned to
   Postgres** — swapping `relational.dialect` remains a configuration change, as it is
   everywhere else in this service. The one real constraint the seam imposes is documented in
   §3.2: `UpsertSetExpr` may not reference a target-table column, which is why the window reset
   is hoisted into its own statement instead of a `CASE`.

5. **Severity mapping** (§3.3) — DECIDED as proposed.
6. **Identity in clear in the log stream — DECIDED: in clear.** Same argument `0007` already
   made for the column: hashing would leave a reviewer able to see that *some* identity was
   tried 400 times without knowing which, which is most of the value gone.

### §3.7 Wire/API impact

**None.** No route added, changed or removed. `POST /auth/user/token` keeps the same request
body, the same `401` for every refusal, the same `429` with the remaining window when locked,
and the same uniform response time. The lockout's *observable* behaviour is unchanged except
for the window-boundary nuance in §3.2. `LockoutThreshold = 5` and `LockoutWindow = 15min` keep
their current values.

## §4 External contract

`N/A — no external system.` Everything here is this service's own database and its own stdout.

## §5 Impact map — every artifact touched

| Artifact | Change | Owning doc section |
|---|---|---|
| `migrations/postgres/0007_authentication_attempts_manual.up.sql` | **rewritten in place** (maintainer-directed). New shape: `id UUID PRIMARY KEY` kept (simple PK — project rule) with `CONSTRAINT authentication_attempts_natural_key UNIQUE (identity, identity_kind, outcome)` as the two-row ceiling, `CHECK (outcome IN ('failure','success'))`, `total_count BIGINT NOT NULL DEFAULT 0`, `current_count INT NOT NULL DEFAULT 0`, `window_started_at TIMESTAMPTZ NULL`, `last_at TIMESTAMPTZ NOT NULL`, `last_ip VARCHAR(45) NOT NULL DEFAULT ''`, `identity_existed BOOLEAN NULL`, `total_blocked BIGINT NOT NULL DEFAULT 0`. Drops both time-ordered indexes — the natural-key unique index serves the only remaining query. Header comments rewritten to state the new bargain honestly — including what moved to the log stream and why. | `migrations.html`, `table-schema.html` (column shapes) |
| `migrations/postgres/0007_authentication_attempts_manual.down.sql` | rewritten: same `DROP TABLE`, but the "this destroys evidence" warning is now materially weaker (the evidence is in the log stream) while "this silently disables the lockout" is unchanged. | `migrations.html` |
| `internal/infra/authentication_attempts_manual.go` | `insertStmt` → a `Dialect.BuildUpsert` pair (with/without `identity_existed`) plus the stale-window reset `UPDATE`, all assembled at construction; `countStmt` → single-row PK probe; `LockedUntil` reads one row by the natural key instead of iterating N; `RecordSuccess` gains the failure-row reset; `RecordLocked` becomes counter-only (bumps `total_blocked`, touches nothing else). The `outcome*` constant block loses `outcomeLocked` — it is no longer a stored value, only `'failure'` and `'success'` are. Public method set otherwise unchanged. | `architecture.html` (invariant 9), `infra/db/core/dialect.go` (`BuildUpsert`) |
| `internal/application/commands/authentication_commands_manual.go` | new one-method publisher port beside `AttemptRecorder`; `IssueTokenHandler` gains the field; four publish sites (locked-refusal, unknown-identity/store-error, wrong-password, unusable-account, success). No change to the refusal branches themselves. | `domain-events.html` |
| `bootstrap/authentication_feature_manual.go` | the feature builds `events.NewSlogPublisher(d.Logger)` and hands it to the handler alongside the attempt store. | `domain-events.html`, `bootstrap.html` |
| `internal/web/authentication_routes_manual.go` | wiring only, if `MountAuthentication`'s signature grows the publisher. | `service-layout.html` |
| `microservice.*.yaml` (both profiles) | **no change** — the slog publisher is wired unconditionally by `bootstrap.Run` and reads no config. | `yaml-reference.html` |
| build/run commands | **unchanged** — `-tags 'postgres kafka'`. No new transport consumer. | `shared/boot-contract.md` |
| notification type(s) + the seven catalogs | `N/A — this change raises no new typed rejection.` The lockout keeps answering with the existing notification and the existing `429`. | `shared/notification-bases.md` |
| `internal/infra/authentication_attempts_manual_test.go` (297 lines) | substantially rewritten: window-boundary reset, the unconditional lifetime total, the success reset, concurrent-increment correctness, the two-row ceiling. | — |
| `internal/infra/attempt_rollup_live_test.go` | **NEW, beyond the original impact map.** `//go:build integration && postgres` — drives the real store against the dev bench through the real engine, so the upserts are rendered by the framework's own pg dialect rather than by a stub. It is the only place the statements are executed instead of inspected, and the only thing that proves the rendered SQL and the schema agree. Excluded from the normal suite by its tag. | `migrations.html` (integration-test convention) |
| `internal/infra/refresh_token_store_manual_test.go`, `internal/infra/authentication_reader_manual_test.go` | test doubles only: `recordingRows` learns to scan `*int`/`**time.Time` and to fail mid-scan and mid-iteration; `testDialect.NowExpr()` returns `NOW()` instead of panicking, now that a statement needs it. | — |
| `internal/application/commands/authentication_commands_manual_test.go` (1012 lines) | the `fakeAttempts` double gains nothing (port unchanged) but a `fakePublisher` is added; every sign-in branch asserts the event it publishes — **and asserts no event ever carries the password**. | — |
| `README.md` lines 51–52 | the "Brute-force lockout" and "Authentication forensics" rows describe the *append-only log* as the mechanism. Both must be rewritten to describe the rollup + the log stream, or the README states something false. | — |

Phase 2 edits ONLY these rows, in dependency order (migration → infra store → application port
and handler → bootstrap/web wiring → tests → README).

## §6 Config & secrets

`N/A — no new configuration and no secrets.` The publisher needs no yaml (`bootstrap.Run`
wires `events.NewSlogPublisher(logger)` unconditionally at `bootstrap.go:569`); the counter
policy stays as Go constants (`LockoutThreshold`, `LockoutWindow`) exactly as today.

## §7 Verify step — how this will be PROVEN

**Build/vet/test:** `go build -tags 'postgres kafka' ./... && go vet -tags 'postgres kafka' ./...
&& go test -tags 'postgres kafka' ./... -count=1`, with the project's 95% coverage floor held.

**The database conformance step (maintainer-directed, run by hand).** Verified live: the DB is
up (`authcore-dev-postgres`, port 5432), `omnicore_migrations` holds exactly one row at
`version=8, dirty=f`, and `authentication_attempts` holds 23 dev rows.

Because `omnicore_migrations` stores **only** `(version, dirty)` — one row, **no per-version
rows and no file checksum** (verified in `migrations.html` and against the live table) —
editing `0007` in place triggers **no** boot abort and **no** re-run. The tracking table must
therefore be **left untouched at version 8**. Rewinding it to 6 would make boot replay `0007`
*and* `0008`, and `0008` would fail against its existing tables and persist `dirty=true`.

So the conformance procedure is exactly the one the request describes, and nothing more:

1. `DROP TABLE "authentication_attempts";` (23 dev rows, no production data)
2. run the **new** `0007_…up.sql` by hand against `authcore_db`
3. leave `omnicore_migrations` at `version=8, dirty=f`

A **fresh** database is unaffected and self-consistent: it runs `0001…0008` with the new `0007`
and lands on the same schema. **This is the one step whose correctness depends on `0007` never
having shipped to an environment with real data** — true today, and the reason it is safe to
edit in place rather than add `0009`.

**Capability proof — EXECUTED, and here is what it showed.**

The proof was run at the SQL layer rather than over HTTP, and deliberately so: it drives the
real `AuthenticationAttemptStore` against the real dev Postgres through the real
`postgres.NewPostgres` engine, so every upsert is rendered by the framework's own pg dialect.
A stub cannot prove that the rendered SQL and the schema agree; this does.
(`internal/infra/attempt_rollup_live_test.go`, `go test -tags 'integration postgres kafka'` —
both tests PASS.)

1. Four failures for one identity → **exactly one row**, `total_count=4`, `current_count=4`,
   `identity_existed=true`, `last_ip` recorded; the probe says **not locked**.
2. The fifth → the probe says **locked**, and the expiry it derives sits within a second of
   `window_started_at + 15min` — asserted against the anchor read back from the row, so a
   stored-and-drifting expiry would fail here.
3. Three attempts hammered **through** the lock → `total_blocked=3`, while `current_count`,
   `total_count` **and the window anchor are all unmoved**. THE LOCK WAS NOT EXTENDED, asserted
   directly. Still one row.
4. A failure carrying an unknown existence verdict → the stored `true` **survives**; the
   nil-answer statement really does leave the column alone.
5. A successful sign-in → `current_count=0`, `window_started_at=NULL`, the probe releases, and
   `total_count` is **unchanged** (history not erased). A second row appears with
   `outcome='success'`, `total_count=1`, `last_at` set, and no lockout machinery on it.
6. **`SELECT count(*)` for that identity → exactly 2.** The request's actual acceptance
   criterion.
7. Separately: a window aged past its expiry → the probe releases, and the next failure restarts
   `current_count` at **1** while `total_count` keeps climbing.

The credential rule is asserted in the unit suite rather than here, on both surfaces: no write
binds a value containing the password (`TestWrites_CannotCarryACredential`) and **no
announcement on any branch carries it** (`TestIssueToken_NoAnnouncementCarriesTheCredential`,
which drives all five branches with a real secret and greps every payload value and message).

**The database conformance step — DONE.** The 23 dev rows were snapshotted first
(`authentication_attempts_pre_0007_rewrite.sql` in the session scratchpad; all 23 dated
2026-08-27, i.e. test noise), then `DROP TABLE` + the new `0007` ran in one transaction.
`omnicore_migrations` was left untouched at `version=8, dirty=f`, exactly as reasoned above.

**THE DEFECT THIS PROOF MISSED, and why — the most important line in this document.**

Everything above ran the sequence **failures → success**. Nothing ran **success → failures**,
which is the order a real account actually lives in: mistype once, sign in, come back later and
start failing. In that order the success NULLs the anchor, the re-anchor predicate's bare
`<=` comparison skips the row, and the identity never locks again — with `current_count` visibly
climbing in the table the whole time, which is what made it so quiet.

The maintainer hit it within minutes of manual use: 18 wrong passwords, no 429. The table showed
`current_count = 18` beside `window_started_at = NULL`.

Two things made the proof blind to it, and both are worth naming:

- **The test suite encoded the bug as a requirement.**
  `TestFailureUpsert_BumpsBothCountersAndNeverMovesTheAnchor` asserts the anchor is absent from
  the upsert's update set — correct on its own, but it never asked what *else* writes that
  column, or whether that writer covers every state.
- **The live proof exercised the design's happy path, not the user's path.** A sequence written
  from the design will re-check what the design already says; only a sequence written from how
  the thing is USED finds where the design is silent.

The regression tests added with the fix were verified **against the reverted, buggy code first**
— the unit test fails with *"every identity that ever signed in successfully would become
permanently unlockable"* and the live test with *"the anchor is still NULL after five failures"*
— so they are known to catch it rather than assumed to.

**What is still NOT proven, and the exact step that closes it:**

- **The HTTP path end to end.** Nothing above went through `POST /auth/user/token`, so the
  handler's five announcement sites are proven by unit tests and not by a running service. The
  step: boot (`/omnicore:run`), sign in wrong 6× against a seeded address, and read the
  `"event"` records on stdout.
- **That the log pipeline ingests and retains them.** This service only guarantees the records
  reach stdout. Closing it is a devops step on the observability stack — and it is the step
  that makes §3.1's trade real rather than theoretical. Until it is done, the forensic record
  has left SQL without arriving anywhere queryable.

---

Original checklist, kept for the record:
**Capability proof — executed, not assumed:**

1. Boot the service (`/omnicore:run`), then `POST /auth/user/token` with a wrong password 4×
   against a real address. Assert: `authentication_attempts` holds **exactly one row**, with
   `total_count = 4`, `current_count = 4`, `identity_existed = true`; and **four** `"event"`
   records on stdout at `WARN` carrying the identity, the IP and `outcome: failure`.
2. 5th failure → still one row, `current_count = 5`. 6th → **`429`** with the remaining window,
   the row **unchanged** (proving a locked attempt cannot extend the lock), and one further
   `"event"` record for the blocked attempt.
3. Sign in correctly after the window elapses → failure row's `current_count = 0` with
   `total_count` still at its lifetime value (proving total ≠ resettable), a **second** row
   appears with `outcome='success'`, and one `"event"` at `INFO`.
4. `SELECT count(*)` over the table after the whole sequence → **exactly 2** for that identity.
   That number is the request's actual acceptance criterion.
5. Grep the captured stdout for the test password → **zero hits**. Asserted in the unit suite
   as well, not only observed here.

**What cannot be proven locally:** that the log pipeline (collector → Elasticsearch/Loki)
actually ingests and retains these records — this service only guarantees they reach stdout.
Closing that is a devops step on the observability stack, and it is the step that makes §3.1's
trade real rather than theoretical. Until it is done, the forensic record has moved out of SQL
without arriving anywhere queryable.
