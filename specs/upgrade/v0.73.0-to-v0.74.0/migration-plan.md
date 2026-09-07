# Migration plan — omnicore v0.73.0 → v0.74.0

Status: APPROVED

Service: `authcore` · Build tags: `postgres` (engine from `relational.dialect`; neither
profile declares a `transport:` block, so no transport tag)

Rollback point: `specs/upgrade/v0.73.0-to-v0.74.0/rollback/` — verbatim `go.mod` + `go.sum`
at `v0.73.0`, taken 2026-09-06, byte-identical by `shasum`. Both files were clean in git at
snapshot time, so `git checkout go.mod go.sum` is an equivalent restore.

One release sits in the range, and it carries **one breaking item**: the managed archive
slot is renamed. It is compile-visible almost everywhere it lands, which is what makes this
upgrade cheap to verify and expensive to leave half-done.

The maintainer paired the bump with a **service-side decision the framework does not
require**: renaming the PHYSICAL column `deleted_at` → `archived_at`, edited in place in
the already-applied migration scripts rather than added as a new versioned migration. §3
states what that costs.

## Operational fallout — the five classes, checked

| Class | Present? | Evidence |
|---|---|---|
| (a) required DDL on the service's own tables | **no** by the framework, **yes** by choice | the changelog says "No data migration and no view rebuild… `ArchivedAt("deleted_at")` stays valid". The DDL in §3 is the maintainer's decision, not the release's demand |
| (b) demanded view rebuild | no | no `mongo:` block in either profile — the read side is relational, there is nothing to rebuild |
| (c) framework embedded migration sequence grew | no | `infra/migration` is identical between the two pins (30 `.sql` files, same set) |
| (d) yaml key moved / renamed / arrived mandatory | no | `application/configuration` is byte-identical between the pins; both profiles stay valid as written. The two yaml edits in §7 are comments only |
| (e) shared gRPC proto contract changed | no | the project ships no `.proto` and no `.pb.go` |

## Decision on the one open item

The seven translation catalogs get the label **translated per language**, not left on the
generator's English default:

| catalog | `<Entity>ArchivedAtField` |
|---|---|
| `eng` | `Archived At` |
| `ptbr` | `Arquivado em` |
| `esp` | `Archivado en` |
| `fra` | `Archivé le` |
| `deu` | `Archiviert am` |
| `ita` | `Archiviato il` |
| `nld` | `Gearchiveerd op` |

This is reversible in one line per catalog. §6 explains why the hand edit survives every
future regeneration.

The maintainer then extended the same treatment to the other two managed stamps, which had
been sitting on the generator's English default in every catalog since they were first
written — leaving `ptbr.go` reading `Created At` beside `Arquivado em`. All three now agree
per language:

| catalog | `CreatedAtField` | `UpdatedAtField` | `ArchivedAtField` |
|---|---|---|---|
| `eng` | `Created At` | `Updated At` | `Archived At` |
| `ptbr` | `Criado em` | `Atualizado em` | `Arquivado em` |
| `esp` | `Creado en` | `Actualizado en` | `Archivado en` |
| `fra` | `Créé le` | `Mis à jour le` | `Archivé le` |
| `deu` | `Erstellt am` | `Aktualisiert am` | `Archiviert am` |
| `ita` | `Creato il` | `Aggiornato il` | `Archiviato il` |
| `nld` | `Aangemaakt op` | `Bijgewerkt op` | `Gearchiveerd op` |

14 keys per catalog (7 entities × 2 stamps) across the six non-English catalogs; `eng`
already matched. `prune` confirms all three stamps per entity per catalog as "left alone —
edited by hand", so no regeneration reverts them.

---

## 1. The breaking change — `DeletedAt` is gone from the framework

### How it worked at v0.73.0

The managed archive slot was `DeletedAt`. `TableSchema.DeletedAt(col)` declared the column,
`TableSchema.DeletedAtColumn()` read it back, `domain.Managed.GetDeletedAt()` answered the
stamp, and the FIXED LOGICAL NAME addressed as a string was `"DeletedAt"`. On the wire the
rendered field was `deletedAt`. Sections: `TableSchema`, `Lifecycle map`.

### How it works at v0.74.0

The slot is `ArchivedAt` and `DeletedAt` no longer exists. Renamed across the whole map:
`TableSchema.ArchivedAt(col)` / `.ArchivedAtColumn()`, `ViewNode.ArchivedAtColumn()` /
`.ChildArchivedAtPaths()`, `hydrate.SchemaArchivedAt`, `core.RoleRef.ArchivedAtCol`,
`domain.Managed.GetArchivedAt()`. The fixed logical name `"DeletedAt"` becomes
`"ArchivedAt"` wherever it is addressed as a string — criteria predicates, a `Leg`'s
`Fields` allowlist, `EmbedFields`, the Direct write's reserved-key gate, `ValidateModes`.
On the wire the rendered field becomes `archivedAt`: the JSON key, the filter token, the
`?fields=` projection token and the sort token. Only PHYSICAL deletion still says delete.
Sections: `TableSchema`, `Lifecycle map`.

The verbs were always `Archive`/`Unarchive`, the mode tokens always `ARCHIVE`/`UNARCHIVE`
and the event always `ARCHIVED` — the slot was the last place the framework still said
"deleted", and it said it on the surface the developer types.

### Wire consequence the build cannot see

Every read DTO this service exposes renames its JSON key `deletedAt` → `archivedAt`, and
the same token changes for filters, `?fields=` and sort. **Consumers matching `deletedAt`
break.** No QA suite ships in this repo today, so nothing here asserts the old token; the
change reaches real clients only.

## 2. `go.mod` + `go.sum`

Applied: `GOFLAGS=-mod=mod go get github.com/ClaudioSchirmer/omnicore@v0.74.0` + `go mod tidy`.

## 3. Migrations — the physical column rename, in place

Twelve scripts under `migrations/postgres/` carry `deleted_at`: as a column in `CREATE
TABLE`, in `COMMENT ON COLUMN`, in partial unique indexes (`WHERE "deleted_at" IS NULL`),
and in SQL view bodies. All become `archived_at`, edited **in the existing files** — no new
migration version.

### What that costs, stated plainly

The framework's migration runner keeps no checksum of an applied script, so editing one in
place is not detected — but it is also not re-applied. **Any database already migrated stays
on `deleted_at`** while the schema declarations now say `archived_at`. Under `prd`'s
`autoRun: check` the boot passes (the sequence is unchanged) and the failure surfaces on the
first read, as a Postgres `column "archived_at" does not exist`. Recreating the database, or
running the `ALTER TABLE … RENAME COLUMN` set by hand, is the operator's step and this skill
does not run it.

The maintainer chose this explicitly ("sem se preocupar com versão").

## 4. omnicore-gen specs — three renames, one of them unrelated to this upgrade

The seven specs under `specs/omnicore-gen/` need:

1. `storage.managed.archivedAt: deleted_at` → `archived_at` (the logical key was already
   `archivedAt`; only the physical column value moves) — and the same for every child block
   declaring its own `archivedAt`.
2. Read-join entries naming the counterpart's stamp: `column: deleted_at` → `archived_at`.
3. **Two generator-language renames that already block generation today**, independent of
   the framework bump — `omnicore-gen check` refuses all seven specs on them:
   - top level `delete:` → `removal:` (all seven specs)
   - child block `softRemove:` → `archiveOnRemove:` (8 occurrences: client ×3, group ×1,
     role ×1, user ×3)

   Both are pure key renames — same struct, same shape (`gen/internal/spec/load.go:245-246`).
4. Two further language items that only surfaced once the unknown-key blockers above
   cleared — `check` reports the first failing layer, not all of them at once:
   - `read.managed: [CreatedAt, UpdatedAt, DeletedAt]` → `ArchivedAt` (all seven). The
     managed stamp is addressed by its LOGICAL name, which is exactly what v0.74.0 renamed.
   - `removal.root: soft` → `archive` (all seven). The removal vocabulary is
     `archive | delete | both` (`gen/internal/spec/vocab.go:159`); `archive` is the same
     semantic the specs already declared — reversible, the row stays.
5. Prose inside the specs that still says `deleted_at`.

## 5. Regeneration

`omnicore-gen` at plugin 0.64.0 targets framework `v0.74.0` exactly
(`gen/internal/compat/compat.go:20`), so before the bump every spec evaluated as `behind`
and generation was blocked. After §2 and §4 the seven entities regenerate, rewriting the
372 `owned` files in the lock.

## 6. Files the regeneration does NOT touch

The lock records **no adopted file** — no `adjustedFor`, no `why`, anywhere. The 15
non-`owned` files are class `hook`: written once if absent, then never touched and never
hashed, because their content is the author's from the moment they exist. They are
therefore mine to edit here.

Of the 15, five mention the old vocabulary and only one is code:

| file | line | what |
|---|---|---|
| `internal/infra/role_service_manual.go` | 142 | `permission.GetDeletedAt()` → `GetArchivedAt()` — **compile break** |
| `internal/domain/tenant_rules_manual.go` | 60 | comment |
| `internal/domain/client_rules_manual.go` | 136 | comment |
| `internal/domain/user_rules_manual.go` | 82 | comment |
| `internal/infra/claim_service_manual.go` | 204 | comment |

Beyond the hooks, 20 hand-written files outside the lock carry the old names: 13 schemas
written by hand (`sign_in_account`, `sign_in_client`, `refresh_token_direct`,
`authentication_attempt_direct`, `held_claim_value`, `held_client_claim_value`,
`claim_definition`, `client_allowed_range`, `client_claim_edge`, `user_claim_edge`,
`client_role_grant`, `user_role_grant`, `user_group_grant`), the two authentication readers
and their tests, and `internal/infra/schemas/*_test.go` calling `DeletedAtColumn()`.

**The translation catalogs are class `registration`** — the generator inserts and removes
keys without rewriting the file, so regeneration itself drops `<Entity>DeletedAtField` and
adds `<Entity>ArchivedAtField`. It writes the English default `Archived At` in all seven,
the same way `CreatedAtField` reads `Created At` in `ptbr.go` today: `read.managed` has no
`text:` seat, while `joins[].fields[].text` does (which is why `RoleTenantArchivedAtField`
is already `Inquilino arquivado em`). Translating the value by hand STICKS —
`MergeMapEntries` compares the on-disk value against the hash the generator recorded and
keeps the author's when they differ (`gen/internal/emit/registration.go:278-292`).

## 7. Prose

Comments in `microservice.dev.yaml` and `microservice.prd.yaml` (three lines, all
explaining the `relational.clock` decision), and the `deleted_at` mentions in
`specs/scaffold-entity/**` and `specs/evolve-entity/**`.

## 8. The stale catalog keys — `prune`, not a rename

Applying §6 by hand exposed a trap worth recording. The catalogs are `registration` files:
regeneration INSERTS `<Entity>ArchivedAtField` but does not remove
`<Entity>DeletedAtField`, because removing what an earlier shape of the spec left behind is
`prune`'s job, not `generate`'s. Renaming the old key in place therefore produces a
DUPLICATE map key — the new one is already there — and Go refuses to compile it.

The correct sequence is: let `generate` insert the new key, DELETE the old line, translate
the new value, then `omnicore-gen prune -apply` per spec so the lock forgets the 49 stale
registrations (7 keys × 7 catalogs) that `doctor` would otherwise keep reporting.

`prune` also confirms the hand translations are safe: it lists the six non-English
`<Entity>ArchivedAtField` entries as "left alone — it was edited by hand since it was
written", and `eng` is absent from that list precisely because its value still matches what
the generator wrote.

## 9. Verify

`go vet -tags postgres ./...` and `go build -tags postgres -o ./bin/authcore ./bootstrap`
(the `./...` build form fails in this project on a pre-existing layout property: the
produced binary name collides with the `bootstrap/` directory), plus the unit suite.

Result, 2026-09-06 — all green:

```
gofmt -l ./internal ./bootstrap                        → no output
go vet   -tags postgres ./...                          → exit 0
go build -tags postgres -o ./bin/authcore ./bootstrap  → exit 0
go test  -tags postgres ./...                          → every package ok
omnicore-gen doctor                                    → seven entities, framework v0.74.0,
                                                         no drift
```

142 files changed. Regeneration updated 110 files across the seven entities and created
none.

---

## Needs the maintainer's attention — not auto-fixable

- **The wire rename reaches clients.** `deletedAt` → `archivedAt` in JSON, filters,
  `?fields=` and sort. Nothing in this repo asserts the old token; every consumer of this
  IdP does.
- **Databases already migrated keep `deleted_at`** (§3). The `ALTER TABLE … RENAME COLUMN`
  set, or a recreate, is an operator step this plan does not perform.
- **Other labels still read English in the non-English catalogs.** Translating the three
  managed stamps exposed how many neighbours are in the same state: comparing `eng.go`
  against `ptbr.go` leaves 26 keys identical after this run. Some are correct as they stand
  (`Workspace`, `E-mail`, `Claim`); others are plainly untranslated — `Full Name`,
  `Given Name`, `Family Name`, `Secret Hash`, `Previous Secret Hash`,
  `Grace Period Seconds`, `Requesting Client ID`, `Requesting Identity Kind`,
  `Tenant Status`, and the bare entity names `Tenant`/`Role`/`Group`/`User`/`Client`/
  `Permission`/`Claim`. Deciding which of those are loanwords and which are gaps is a
  separate pass, per key, and was not part of this run.
