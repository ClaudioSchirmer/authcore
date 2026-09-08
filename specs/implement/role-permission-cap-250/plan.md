# Capability plan — role-permission-cap-250

- **Status:** APPROVED — 2026-09-08
- **Framework pin:** `github.com/ClaudioSchirmer/omnicore v0.74.0` (latest published — no upgrade in play)
- **Generator:** omnicore-gen, plugin 0.66.0 (installed = published)

## §1 The request (restated)

> quero subir o teto de permissions em role para 250, e quanto a geracao do token, usar uma
> coleção "set" ou algo do tipo para NUNCA duplicar. Uma permission deve sair uma vez só na
> coleção do jwt, seja de client ou de user. Pós concluir verificar os testes de QA que existem
> e documentação. Alinhar tudo.

When this is done, a role may grant **250** permissions instead of 200 — one number in the
Role spec, regenerated into the aggregate's rule, and every place in the repository that
quotes the old figure corrected in the same round. The second half of the request is
**already true and already proven**: both token paths collapse duplicates through a set
before the claim is built. What this plan proposes for it is not a new mechanism but the
removal of a drift risk — the collapse is currently hand-copied into two files — plus the
end-to-end proof the QA suite does not yet carry.

## §2 Routing evidence — the owning docs

| Capability piece | Owning section(s) at this pin | Existence check |
|---|---|---|
| The per-collection cap and its typed rejection | `rules-dsl.html` — a rule rejects by field reference; `r.AddNotificationNamed("Permissions", n, value)` is the explicit-string seat for a cross-field / collection rejection | present at pin; already in use at `internal/domain/role.go:153` |
| The 422 the cap answers with | `status-mapping.html` — the notification's `Semantic()` decides the status | present; `TooManyPermissionsInRoleNotification` already mapped |
| The `permissions` claim in a minted token | `token-issuance.html:25` — *"`TokenRequest.Claims` is where groups, permissions, `tenant_id` … ride — **the framework never interprets it**, that vocabulary belongs to the issuing service's own domain"* | present |
| How a consumer reads that claim back | `authz-seams.html:32,165` — `resource:action`, three match rules, and *"the parsed permissions set is **cached on the `Identity`** after the first call"* | present |
| The readers that assemble the bundle | `direct-schema.html` — the Direct anchor both sign-in readers hang off | present; already in use |
| The `cap:` key in the entity spec | generator spec language (`omnicore-gen explain`), `specs/omnicore-gen/role.omnicore.yaml` `rules.list[permission-cap].cap` | present; declared at line 632 |

**Routing outcome: offered at pin.** Two consequences worth stating before the gate, both
read off §2 rather than assumed:

1. **The dedup is this service's job and nobody else's.** `token-issuance.html` is explicit
   that the framework never interprets the claim. There is no framework "set" to switch on;
   if the service emits a duplicate, it ships.
2. **A duplicate was never an authorization defect.** `authz-seams.html` says the consumer
   parses the claim into a **set** and caches it on the `Identity`. A repeated entry would
   have cost header bytes and readability, never a wrong allow/deny. This is context for
   sizing the work, not an argument against doing it.

### Routing note — why this plan sits in `implement` and not `evolve-entity`

The cap is a `rules.list[].cap` value. Changing it moves **no** column, DTO, migration,
OpenAPI shape or translation string (the seven catalogs interpolate `{max}`; the number
never appears in them). `evolve-entity` owns schema evolution, and there is none here.
**Redirect this at the gate if you would rather it went through that skill** — the edit is
the same either way.

## §3 Integration semantics [high-risk — propose + CONFIRM]

- **Seam:** none new. The cap stays where it is — inside `Role.BuildRules`, `IfInsertOrUpdate`,
  generated from the spec. The dedup stays where it is — inside the two sign-in readers, before
  the bundle is returned.
- **Sync or async:** N/A — both are in-flow, in the request that already runs.
- **Failure policy:** N/A — no external dependency is added or called.
- **Idempotency / replay:** N/A — no event, no receiver.
- **Cache slots:** N/A — nothing cached by this change.
- **Wire/API impact — the one thing that IS wire-visible.** `TooManyPermissionsInRoleNotification`
  carries its limit to the caller (`Max`), and the 422 body will change from `200` to `250`.
  Any consumer asserting the old figure breaks; inside this repository that consumer is the QA
  suite, corrected in §5. No route, field or status code moves.
- **Header budget — the honest arithmetic.** The cap's stated second job is a claim-size
  budget. One rendered entry is at most 129 runes (`vos/permission_key.go`, `maxRenderedRunes`),
  so the worst case **one role** contributes to a token grows from ~25.8 KB to ~32.3 KB. It does
  **not** multiply by Group's 50: the claim is the deduplicated UNION, so a principal's real
  ceiling is the size of the live catalog, which nothing bounds. Raising this number raises what
  a single role can cost; it does not create a new ceiling, because there was never one.

## §4 External contract

N/A — no external system.

## §5 Impact map — every artifact touched

### D1 — the cap, 200 → 250

| Artifact | Change | Owning doc section |
|---|---|---|
| `specs/omnicore-gen/role.omnicore.yaml` | `rules.list[permission-cap].cap: 200` → `250`; the rule `description`'s figure; the `PermissionIsNotInCatalog` fact comment at L731 (`pays 200 round trips` → 250) | generator spec language |
| `internal/domain/role.go` | **regenerated** — the `len(items) > 200` guard, its comment, `Max: "200"` | `rules-dsl.html` |
| Role's other generated files | regenerated; expected byte-identical (nothing else reads the cap) | — |
| `internal/domain/role_rules_manual.go` | L69 comment `200-permission cap` → 250 (hand-written; the generator does not own it) | — |
| `internal/infra/role_service_manual.go` | L32 comment `at the 200-permission cap therefore pays 200 round trips` → 250 | — |
| `internal/domain/role_children_manual_test.go` | `roleWithGrants(200)`/`(201)` → `(250)`/`(251)` and the two failure messages. `uuidForIndex` yields 4096 distinct ids, so 251 is inside its range | — |
| `internal/domain/group_children_manual_test.go` | L47 comment `lower than Role's 200` → 250 | — |
| `qa/domain.sh` | RL8 section header; the negative fixture `seq 1 201` → `251`; the asserted `value` `"201"` → `"251"`; the two `case_` prose lines | — |
| `specs/qa/role-contract/plan.md` | L456 RL8 row — the cap figure and the two 201s | — |
| `specs/scaffold-entity/role/spec.md` | L83, L469, L493, L685, L690 — the figure, with a dated supersession note rather than a silent overwrite | — |
| `internal/domain/group.go` + `specs/omnicore-gen/group.omnicore.yaml` L653 | `multiplies against Role's own 200` → 250 — **gated by D4 below** | — |
| notification type(s) + translation catalogs | **no change** — `TooManyPermissionsInRoleNotification` interpolates `{max}`; all seven catalogs are correct as they stand | `shared/notification-bases.md` |
| `microservice.*.yaml`, bootstrap, build tags | **no change** — the cap is a domain constant, not config | `shared/boot-contract.md` |

### D2 — the token's permission set

**Finding first, because it changes what there is to build.** The collapse already exists on
every path a token is minted from, and it is already proven:

| Path | Where the set lives | Proof today |
|---|---|---|
| User login | `internal/infra/authentication_reader.go` — `assemble()`, `seenPerm map[string]struct{}` keyed on `resource:action` | `authentication_reader_live_test.go` — the fixture grants `report:read` through **both** a direct role and an inherited one and asserts the answer is *exactly* `["invoice:read","report:read"]` |
| User refresh | same — `refresh_token_command_handler.go:84` re-resolves through `ResolveSignIn` | covered by the same collapse |
| Client credentials | `internal/infra/client_authentication_reader.go` — `assembleClient()`, its own `seenPerm` | `client_authentication_reader_test.go:238-260` — *"expected 2 deduplicated permissions"* |

Both also `sort.Slice` the result, so two tokens minted from the same grants are byte-identical.
`EffectivePermissions` → `RenderPermissions` only renders what these produced; no later step can
reintroduce a repeat.

So the proposal is **not** a new set. It is these two rows:

| Artifact | Change | Owning doc section |
|---|---|---|
| `internal/infra/permission_set_manual.go` (new) | one unexported `permissionSet` in `package infra` — `add(resource, action *string, grantArchivedAt, permissionArchivedAt *time.Time)` applying the three gates and the collapse, and `sorted() []vos.PermissionKey`. Both readers delegate to it, so the guarantee has ONE implementation instead of two hand-copied ones | `service-layout.html` — infra; both callers already live there |
| `internal/infra/authentication_reader.go`, `client_authentication_reader.go` | `assemble` / `assembleClient` drop their local `seenPerm`, `addPerm` and `sort.Slice` and call the shared collector. No behavior change — same gates, same order, same output | `direct-schema.html` |
| `internal/infra/permission_set_manual_test.go` (new) | unit cover for the collector: the repeat collapses, each of the three gates drops its row, the order is stable, an empty input answers non-nil | 95% floor |
| `qa/domain.sh` | a new case, end to end: one user granted the same catalog row through a **direct role** and through a **group role**, asserting the minted claim carries it **exactly once** — the guarantee the suite reads today (`jwt_claim … permissions`) but never asserts for uniqueness | — |
| `specs/qa/role-contract/plan.md` (or the group lane's plan, whichever the new case sits beside) | record the added case | — |

**Why bother, stated plainly, since the behavior is already correct:** the two collapses are
independent hand-written copies of the same rule. Nothing today makes them fail together, and
the client copy has no live test — only a unit one. The maintainer's sentence *"seja de client
ou de user"* is exactly a request that they cannot diverge.

### D3 — alignment sweep (mandatory follow-through of D1)

Every site is listed in D1's table. Checked and found **clean** (no change needed):
`ACCESS_MATRIX.md` (quotes no cap), `README.md`, the six other entity specs, the seven
translation catalogs, all migrations, `microservice.*.yaml`.

## §6 Config & secrets

N/A — no key, no secret, no profile touched.

## §7 Verify step — how this will be PROVEN

1. `omnicore-gen doctor` clean for **Role** before and after (it is clean now; see D4 for Group).
2. `go build -tags 'postgres' ./... && go vet -tags 'postgres' ./... && go test -tags 'postgres' ./... -count=1` — green.
3. **The cap actually moved:** `TestARoleAtTheCapIsAccepted` at 250 and
   `TestARoleOnePastTheCapIsRefused` at 251, both green — the second is what proves 250 binds
   rather than merely being written down.
4. **The set holds:** the new collector's unit tests, plus the existing live test
   (`go test -tags 'integration postgres'`, needs the dev Postgres up) still asserting
   `report:read` exactly once across two paths.
5. **End to end:** `qa/run.sh` green, including the corrected RL8 (251 → 422 carrying `"251"`)
   and the new no-duplicate case reading the real minted JWT.
6. **Cannot be proven without infrastructure:** the live and QA lanes need the dev Postgres and
   a booted service. If it is not up, those two are reported UNVERIFIED with the exact command
   to close them, never as passes.

## §8 Verification record — 2026-09-08

Nothing below is reported from inference; each line is a command that ran.

| Step | Command | Result |
|---|---|---|
| Generator drift | `omnicore-gen doctor` | **clean for all seven entities** — Group's pre-existing drift closed by D4(a) |
| Role regeneration | `omnicore-gen generate -spec …/role.omnicore.yaml` | 1 updated (`internal/domain/role.go`), 47 unchanged, 5 kept as-is. The diff is the guard, the comment and `Max` |
| Group regeneration | `omnicore-gen generate -spec …/group.omnicore.yaml` | 1 updated (`internal/domain/group.go`), 47 unchanged, 4 kept as-is. **Comment-only**, as the plan predicted — nothing structural rode along |
| Build + vet | `go build/vet -tags 'postgres' ./...` | clean |
| Unit tests | `go test -tags 'postgres' ./... -count=1` | all packages ok |
| The cap binds at its NEW edge | `TestARoleAtTheCapIsAccepted` (250) · `TestARoleOnePastTheCapIsRefused` (251) | both pass |
| The collector | 8 tests in `permission_set_manual_test.go` | pass |
| The collapse still holds against a DATABASE | `go test -tags 'integration postgres' -run TestLive_TheGrantWalk -v` | `TestLive_TheGrantWalkAnswersEveryPathAndNoRevokedOne` **PASS**, `…DropsEachRetiredThingForItsOwnReason` **PASS** — ran, not skipped |
| End to end | `./qa/run.sh` | **ALL GREEN — 17/17 suites · 2224 cases · 75s**, 33 pre-existing skips, none of them new |

The five changed/added QA cases, read back from `qa/.logs/20260908-124311-39992/domain.log`:
`RL8- an insert carrying 251 distinct ids` ✔ · `RL8- the echoed value is the COUNT SENT` (251) ✔ ·
`RL8-3 the LIMIT rides in the interpolated message` (250) ✔ · `GR14.1`–`GR14.5` ✔.
`GR14.1` passing is what makes the rest meaningful: it proves the fixture's principal really did
reach `tenant:read` through **two** paths, so `GR14.2` measured a collapse rather than a
principal that never had a duplicate.

### One correction made in passing

`qa/domain.sh`'s third RL8 case was titled *"the cap notification hands back the limit itself"*
while asserting the **count sent**. The rule exposes `len(items)`, so that assertion could never
have detected the cap moving. It is retitled to say what it checks, and the assertion the old
title described now exists as `RL8-3` — the interpolated message, which is the only place the
cap's own number reaches a caller.

---

## Open decisions — the gate

### D1 — the cap number and its prose — **DECIDED 2026-09-08: as proposed**
`250` taken as given. The rule's reasoning text keeps its current shape (*"Sized to this
platform rather than to GCP's 3000 or Azure's 2000, and it doubles as a claim-size budget"*)
with the figure swapped, rather than being rewritten.

### D2 — what to do about a guarantee that already holds — **DECIDED 2026-09-08: (c)**
- (a) Close it as already done. No code change; report the evidence.
- (b) Extract the shared collector, so the user and client paths cannot drift.
- **(c) CHOSEN** — (b) **+ the QA case** proving no duplicate reaches a real JWT.

### D3 — the `role/spec.md` corrections — **DECIDED 2026-09-08: as proposed**
The figure is corrected with a dated supersession note beside it, matching how this repository
records superseded decisions elsewhere — never a silent overwrite.

### ⚠️ D4 — Group's PRE-EXISTING generator drift, which this change walks into
`omnicore-gen doctor` reports today, before anything in this plan runs:

> `Group (spec specs/omnicore-gen/group.omnicore.yaml, framework v0.74.0)`
> `! the spec changed since the last generation — regenerate to bring the code back in line`

The cause is commit `6fc1861` (*"Add the group-contract QA suite and its plan"*), which landed
**prose-only** corrections in the Group spec's descriptions without regenerating. It is
unrelated to this request. It matters here only because `group.omnicore.yaml:653` says *"this
cap multiplies against Role's own 200"*, and that sentence is generated into
`internal/domain/group.go:155`.

**DECIDED 2026-09-08: (a).**

- **(a) CHOSEN** — Regenerate Group in this round. The comment lands correctly, and the four
  pending prose corrections land with it — deliberate work already approved in the spec, but
  work this request did not ask for. The Group diff is therefore expected to be **comment-only**;
  anything structural in it is a finding to report, not to accept silently.
- (b) Update the Group spec text only, and leave regeneration for its own round. `group.go`
  keeps saying `200` until then, and the drift widens by one line.
- (c) Leave the Group spec alone entirely. Nothing drifts further; the sentence stays wrong.

### Consequences for §5 and §7 of the two decisions above

- D2(c) makes every row of D2's table live: the new collector, both readers rewired, the
  collector's unit tests, and the QA no-duplicate case.
- D4(a) adds `internal/domain/group.go` and Group's other generated files to the impact map as
  **regenerated**, and adds one line to §7: `omnicore-gen doctor` must come out clean for
  **Group as well as Role**, which it is not today.
