# Spec: Permission — notification naming, echoed value, and the `resource:action` wording

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-03, at the gate
- **Pin:** omnicore **`v0.72.0`** · dialect postgres · relational-served views
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md` rule 3
- **Generation:** omnicore-gen — chosen at the gateway (§9), option 1
- **Branch:** `refactor/permission-notification-naming`
- **doctor at start:** clean — no hand-edited generated file, no adopted file, no spec drift,
  nothing missing. The whole tree tracks its spec.

---

## 1. The change, in one paragraph

The `Permission` composed value object stays **exactly as it is** — same two parts, same
rules, same columns, same wire shape. Three things about how it *reports* change. First, the
two notifications the entity raises about the pair name the field **`key`** on the wire; the
pair is called a *permission* everywhere else in this service (the read field is
`permission`, the label renders "Permission"/"Permissão"), so `key` is a name nothing else
uses and no request or response carries. It becomes **`permission`**. Second, of those two
notifications only the immutability one echoes the offending value back; the duplicate one
(`permission.go:107`) answers with `value: null`. **Both** must echo the current pair.
Third, the end-user texts must make the composition explicit — **a permission is
`resource:action`** — so that a caller reading a rejection knows whether the complaint is
about the resource half, the action half, or the pair as a whole.

## 2. Impact map

Ownership is marked because it decides who writes each line on the codegen path:
**[G]** generator-owned · **[M]** a `_manual` hook the generator wrote once and never
touches · **[H]** hand-written on either path.

### The spec YAML — the change itself on the codegen path
| File | What moves |
|---|---|
| `specs/omnicore-gen/permission.omnicore.yaml` **[H]** | `fields[0].name: Key` → `Permission`; `fields[0].unique.echoValue: true`; `rules.list[key-immutable].fields: [Permission]`; `update.patchExcludes: [Permission]`; the six `notifications[].text` blocks (§6); the field/VO `description:` prose that still says "key" |

### Generator-owned — regenerated from the YAML, never hand-edited
| File | What moves |
|---|---|
| `internal/domain/permission.go` **[G]** | struct field `Key` → `Permission`; `AddNotification("Key", …)` → `("Permission", …)` on **both** rules (l. 90, l. 107); l. 107 gains the echo `e.Permission.String()` |
| `internal/infra/permission_repository.go` **[G]** | constraint binding `Field: "key"` → `"permission"` (the DB-constraint path must answer the same field name as the service pre-check) |
| `internal/infra/schemas/permission_schema.go` **[G]** | nothing — `Composite(...)` names the **parts** (`Resource`, `Action`), never the field. Columns untouched. |
| `internal/application/commands/insert_permission_command.go` **[G]** | `e.Key` → `e.Permission` (3 sites) |
| `internal/application/commands/patch_permission_command.go` **[G]** | `e.Key` → `e.Permission` (3 sites) |
| `internal/application/translations/{eng,ptbr,esp,fra,deu,ita,nld}.go` **[G]** | the six notification texts; plus the `PermissionKeyField` label row, per the open item §6.b |
| the generator's own `_test.go` files **[G]** | regenerated |

### Hand-written on either path
| File | What moves |
|---|---|
| `internal/domain/permission_rules_manual.go` **[M]** | `e.Key.String()` → `e.Permission.String()`; the rule's own prose says "key" throughout and is reworded to "permission" |
| `internal/infra/role_service_manual.go` **[H]** | l. 143–144 `permission.Key.Resource` / `.Action` → `permission.Permission.…` — **another entity's** hand-written file reaching into this one |
| `internal/domain/permission_test.go`, `permission_rules_manual_test.go`, `internal/application/commands/{insert,patch}_permission_command_test.go`, `internal/infra/schemas/permission_schema_test.go` **[G/H]** | field renames + the two changed assertions (§7) |
| `qa/permission.sh` **[H]** | cases **E1c**, **E1d** and the D-block comments (§7) |

### Added mid-run, at the maintainer's request
| File | What moves |
|---|---|
| `specs/omnicore-gen/permission.omnicore.yaml` **[H]** | a new `docs:` block — `description` plus `operations` for `insert` / `patch` / `byParams` |
| `internal/web/permission_routes.go` **[G]** | the OpenAPI `Doc.Description` of all five REST operations |

The gate's §5 said no OpenAPI text moved. The maintainer's answer was that the point of the
change is lost if the composition is only ever explained *after* a rejection: *"queria ao
menos no swagger"*. `docs:` is the one block of this spec whose prose reaches the OpenAPI
document, and the entity declared none — which is why the first regeneration left the
endpoint documentation untouched. It is purely additive: prose appended to five operation
descriptions, no schema, no parameter, no status code.

**GraphQL has no equivalent seat at this pin, and that is a framework fact, not a generator
one.** Checked three ways rather than asserted: `explain keys` exposes no GraphQL docs key;
nothing in `omnicore/web/graphql/` ever assigns a `Description` on the schema AST; and
`introspection.go` reads `def.Description` / `f.Description` from a producer that does not
exist. So the SDL and GraphiQL render descriptionless at v0.72.0 for every entity of this
service, and nothing this change could declare would alter that.

### Deliberately NOT touched — and why
- **No migration.** The physical columns are `resource_name` / `action_name`, declared under
  `parts[].column`; the Go field name reaches no DDL. The unique index
  `permissions_resource_name_action_name_key` is Postgres' own name derived from the
  **columns** and is unchanged, so the binding key in the repository stays byte-identical.
- **No view `Version` bump.** `read.backing: relational` — the view composes at read time,
  materializes nothing and carries no version. Nothing to rebuild.
- **`internal/domain/vos/permission_key.go` unchanged.** The VO keeps its type name, its
  parts, `String()`, and every notification it raises on `Resource` / `Action`. The user's
  instruction is explicit: the composed VO stays exactly as it is. Only its **texts** move
  (§6), and those live in the catalogs, not in this file.
- **`microservice.*.yaml`, the proto contract, the GraphQL schema, other entities' views** —
  no route path, no published shape and no projection moves.

## 3. Migration strategy

**N/A — no DDL.** See §2. No column, no constraint, no index and no stored description
changes. The `COMMENT ON` texts already in the database describe the table and its columns
(`resource_name`, `action_name`, `description`); none of them is reworded by this change.

## 4. View evolution

**N/A.** `read.backing: relational`. No materialized view, no `Version`, no rebuild, and no
`JoinView` embedder of this entity exists. The read shape is untouched: `permission` is a
`read.computed` field derived from `Resource` + `Action` and it neither gains nor loses a
source.

### 3b / 4b
**N/A** — nothing is added. No stamped field, no read join; this change adds no field at all.

## 5. API impact — the breaking part, stated plainly

Three wire-visible changes, all inside the **error envelope**. No request body, no response
body, no filter, no `?fields=` token, no `?orderBy` token and no OpenAPI schema moves.

| # | Where | Today | After | Breaking? |
|---|---|---|---|---|
| 5.1 | 409 duplicate — `messages[].field` | `"key"` | `"permission"` | **Yes** for a consumer branching on the field name. Pinned by QA E1c, which exists precisely to catch this drift. |
| 5.2 | 409 duplicate — `messages[].value` | `null` (elided) | `"tenant:read"` — the refused pair, rendered by `PermissionKey.String()` | **Additive.** A consumer reading `value` gets a string where it got nothing; nobody was relying on the absence except QA E1d. |
| 5.3 | 422 immutable pair — `messages[].field` | `"key"` | `"permission"` | **Yes in principle, unreachable in practice.** `PatchPermissionRequest` declares `description` and nothing else, so REST cannot open the door this rule guards (QA D12 states this). GraphQL mounts the same PATCH shape. |

`messages[].fieldLabel` keeps rendering "Permission" / "Permissão" — that is the point of the
open item in §6.b, not a side effect.

**Recommendation:** take all three. 5.1 is the whole request; 5.2 is the whole request; 5.3
is the same rule reported the same way and leaving it as `key` would put two names on one
concept, which is the defect being fixed.

## 6. Translations

### 6.a — the six notification texts, item by item

The instruction is that the end user should be able to tell **which half** was refused and
that a permission is the composition `resource:action`. Proposed wording per message, ENG
shown; the other six catalogs follow the same sentence.

| # | Notification | Today (eng) | Proposed (eng) | Take? |
|---|---|---|---|---|
| 6.a.1 | `InvalidResourceNameNotification` | The resource must be lowercase slugs joined by colons, or exactly "*". | A permission is `resource:action`; the **resource** half must be lowercase slugs joined by colons, or exactly "*". | **recommended** |
| 6.a.2 | `InvalidActionNameNotification` | The action must be a single lowercase slug, or exactly "*". | A permission is `resource:action`; the **action** half must be a single lowercase slug, or exactly "*". | **recommended** |
| 6.a.3 | `UnmatchablePermissionKeyNotification` | A wildcard resource requires the action to be "*" as well. | A permission is `resource:action`; a wildcard resource requires the action to be "*" as well. | **recommended** |
| 6.a.4 | `PermissionAlreadyExistsNotification` | This permission already exists. | This permission already exists — a permission is `resource:action`, and this pair is already taken. | **recommended** |
| 6.a.5 | `PermissionKeyIsImmutableNotification` | The resource and the action cannot be changed. | A permission is `resource:action`; neither half can be changed after creation. | **TAKEN** — maintainer's call at the gate |
| 6.a.6 | `PermissionDescriptionEchoesKeyNotification` | The description must explain the permission, not repeat it. | — | **NOT TAKEN** — maintainer's call at the gate: this one complains about the *description*, not about the pair. Text unchanged in all seven catalogs. |

The name `PermissionKeyIsImmutableNotification` / `…EchoesKeyNotification` still says *Key*.
Renaming a notification type is a wire break of a different kind — `notificationKey` is what
consumers switch on, and QA D10b/E1b assert it by name. **Not proposed.** Raised here so the
silence is deliberate.

### 6.b — the label key — **ANSWERED: option 2**

The catalogs carry **two keys with identical text** in all seven languages:
`PermissionKeyField` ("Permission"/"Permissão", worn by the entity field) and
`PermissionPermissionField` (same text, worn by the read DTO's `exportLabelKey`). Once the
field is named `Permission`, `PermissionKeyField` is a key named after a field that no longer
exists. Three ways out — this is a **removal of a public surface**, so `CLAUDE.md` rule 2
applies and it is asked, not decided:

1. **Keep `PermissionKeyField` as-is.** Zero catalog churn, zero risk; the key's name lies
   about the field it labels.
2. **Point the field at the existing `PermissionPermissionField` and let `prune` remove
   `PermissionKeyField` from all seven catalogs.** One key instead of two saying the same
   word; it is a deletion from the catalogs.
3. **Rename the key** to a fresh `PermissionPairField` (or similar) and remove the old one.
   Same deletion, plus a third spelling of one concept.

**ANSWERED: option 2** (maintainer, at the gate). `fields[0].labelKey` becomes
`PermissionPermissionField`; `PermissionKeyField` is dropped from the spec and `prune`
removes it from all seven catalogs. This is the one deletion in this change and it was
explicitly authorized.

## 7. Tests

### Unit — mechanical, from the rename
`permission_test.go`, `permission_rules_manual_test.go`, `insert_permission_command_test.go`,
`patch_permission_command_test.go`, `permission_schema_test.go`: `Key:` → `Permission:` in
every fixture. The generator rewrites its own; the `_manual` ones are hand-edited.

### Unit — the two that change on purpose
- the duplicate-pair test asserts the emitted field name and now asserts `permission`;
- a new assertion that the duplicate notification carries the rendered pair as its value —
  the behaviour that did not exist before, so it had no test.

### Contract QA — `qa/permission.sh`, planned here, not discovered later
| Case | Today | After |
|---|---|---|
| **E1c** "the field the envelope names" | asserts `'key'` | asserts `'permission'`; its comment block currently *explains* why the field is called `key` and no request carries that name — the whole paragraph is rewritten, because after this change the field name matches the read field the caller already knows |
| **E1d** "the tuple itself is not echoed back" | asserts `value == null`, with a comment explaining that the generator silences the value on composite uniqueness | **inverted**: asserts `value == 'tenant:read'`. The comment is replaced — that generator limitation is gone at this build, `unique.echoValue: true` is accepted and echoes through `String()` (verified with `omnicore-gen check`) |
| **D12** comment | states `PermissionKeyIsImmutableNotification` guards a door REST cannot open | unchanged in behaviour; the sentence naming the field `key` is corrected |
| **H2a** `?key=x` → 400 `SchemaViolationNotification` field `key` | unchanged | unchanged — that 400 names the **query parameter the caller sent**, which is unknown before and after; the rename makes no `key` filter exist |
| **D1–D11**, **E1a/E1b/E2/E3/E4**, **G**, **H1/H3/H4** | — | unchanged |

These are deliberate contract updates that follow an approved contract change, written into
this plan before the code moves — not tests edited to pass.

## 8. Kind promotion

**N/A** — `storage.kind: flat`, unchanged.

## 9. Generation path — the gateway

Asked in the same message as this gate. The change **is** expressible: verified against this
build with `omnicore-gen check`, not from memory.

- `fields[].unique.attachTo` was tried first and is **refused**: *"`Permission` does not name
  a field of this entity — name one of `fields[]`"*. So the field itself is renamed; that is
  the mechanism, not a preference.
- `fields[].unique.echoValue: true` **validates**, with one warning: *"the conflict will echo
  `PermissionKey` through its `String()` — make sure `PermissionKey` declares one"*. It does
  (`vos/permission_key.go:82`). This is what closes the `value: null` gap.
- The renamed field with `rules.list[].fields: [Permission]` and
  `update.patchExcludes: [Permission]` validates with **no blocker and no collision** against
  the `read.computed` field also named `Permission`.

What lands **by hand on either path**: `permission_rules_manual.go`, `role_service_manual.go`
(another entity's file), `qa/permission.sh`, and the two deliberate unit assertions.

## Final verify — what will be proven
`omnicore-gen check` → `generate` → read the report → `doctor` clean → `prune` clean (it is
what removes a dead translation key if §6.b takes option 2) → `gofmt -l` / `go vet` /
`go build` → unit tests → **a real boot** → `qa/run.sh`.

---

## Verification record — 2026-09-03

| Gate | Result |
|---|---|
| `omnicore-gen check` | ✓ green; one expected warning (`echoValue` echoes `PermissionKey` through `String()`, which `vos/permission_key.go:82` declares) |
| `omnicore-gen generate` | 15 files updated across two runs, 4 kept as-is (the `_manual` hooks and the migration pair) |
| report — migration | *"nothing about the storage changed this run"*; the printed shape and the index `permissions_resource_name_action_name_key` match the existing `0002` pair byte for byte. **No new pair owed** |
| `omnicore-gen prune` | 7 dead `PermissionKeyField` entries removed (the authorized §6.b deletion), then re-run: *"Nothing to prune"* |
| `omnicore-gen doctor` | clean — no hand-edited generated file, no unintended adoption, no spec drift |
| `gofmt -l` · `go vet` · `go build` (tag `postgres`) | clean |
| `go test ./... -count=1` | all green, including the new `permissionEchoed` assertion |
| stale-name grep | no `Permission.Key` reference and no `PermissionKeyField` anywhere in Go, shell or yaml |
| migration pairing | 12 up / 12 down, unchanged |
| **boot + contract QA** | `./qa/run.sh permission` — real Postgres, real boot, **245 GREEN · 0 RED** |

The three targets, proven on the running service rather than by reading the code:

- `E1c` → `field == permission`
- `E1d` → `value == tenant:read`
- `H2a`/`H3` → unchanged; `?key=` and `?permission=` are still typed 400s

### Deviation — one RED, in another lane, and it predates this change

`./qa/run.sh --all` reports **tenant J8** red: `{ __typename }` sent with no bearer expects
401 and answers 200. It is **v0.72.0 upgrade fallout, not this change**, and the proof is
not an inference:

- the changelog for v0.72.0 introduces `graphql.introspection: true`, which *"additionally
  makes an introspection-ONLY POST public"*;
- `qa/microservice.qa.yaml:65` declares exactly that;
- `{ __typename }` **is** introspection-only, so it is now public **by design**;
- the permission lane's own J8 sends a DATA field, stays guarded, and passes 401;
- the upgrade landed at 12:26 today (`4599324`), and the last tenant run was 00:14 — QA was
  never re-run in between.

The assertion is stale, not the service.

**RESOLVED, 2026-09-03**, in two steps, both at the maintainer's direction and both outside
this impact map — recorded here because this run is where they surfaced:

1. **omnicore v0.72.1** fixes `__typename`, which was gated as introspection and resolved by
   nothing (null at the root with an internal error string, silently null when nested). The
   full report and its verification are in `omnicore-typename-defect.md` beside this file.
   Re-verified here: `gofmt`/`vet`/`build`/unit suite/`doctor` all clean on the new pin.
2. **`qa/tenant.sh` J8** became five cases pinning the real boundary — an introspection-only
   POST is public by configuration and answers `Query`; a document reaching DATA is still
   401; a meta field beside a data field smuggles nothing.

`./qa/run.sh --all` — **450 cases, 2/2 suites, ALL GREEN**.
