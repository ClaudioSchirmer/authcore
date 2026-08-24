# Migration plan — omnicore v0.56.1 → v0.57.0

Status: NO EDITS PROPOSED — nothing to approve, nothing was applied

Service: `authcore` · Build tags: `postgres` (engine from `relational.dialect`; neither
profile declares a `transport:` block)

Rollback point: `specs/upgrade/rollback/` (verbatim pre-bump `go.mod` + `go.sum` at
`v0.56.1`, taken 2026-08-23).

> This file records the diagnosis behind the "upgrade + migrate" path. The previous
> record, `migration-plan.md` (v0.54.0 → v0.55.0, APPROVED), is a separate document rather
> than something this run overwrote. Its §1 carries the ordering vocabulary, which has since
> moved: the live answer is `../scaffold-entity/tenant/spec.md` §9, and §1 states what the
> shipped vocabulary costs.

## Outcome of the bump

The bump moved `go.mod` + `go.sum` and nothing else.

```
go vet   -tags postgres ./...                      → exit 0
go build -tags postgres -o <tmp> ./bootstrap       → exit 0
```

Note on the verify command: `go build -tags postgres ./...` fails in this project with
`build output "bootstrap" already exists and is a directory` — the produced binary name
collides with the `bootstrap/` package directory. That is a pre-existing layout property,
independent of any omnicore version; `start.sh:12` uses the `-o ./bin/authcore ./bootstrap`
form for the same reason, and this verify used it too.

## Compile-visible breaking items — all clear

Every symbol v0.57.0 changed or removed on the read side was grepped across the project's
Go sources. `authcore` is still the empty bootstrap shell (`bootstrap/main.go` +
`bootstrap/wire.go`, 46 lines, `Wiring` with no `Features`, no `Views`, no
`Translations`), so none of them is referenced:

| Changed / removed in v0.57.0 | Referenced in `authcore` |
|---|---|
| `ReadCriteria.Projection` / `Page.Projection` → `queries.Projection`; hand-written `queries.ViewReader` now owns its own `_id` → `ID` normalization | no |
| `core.FieldResolver` → answers `core.ResolvedField`; `core.FieldOwner` gains `OwnerJoin` | no |
| `read.AggregateLoader[T].BoundTable()` → `Schema()` | no |
| `queryschema.Read.Projection`, `queryschema.ParseProjection`, `export.Plan.PruneToProjection` | no |
| `query.ViewReaderEngine.SetRelational` → `Register(reader, views)`; `MongoReader()` → `Fallback()` | no |
| `RelationalCapabilityNotification` → `UnsupportedCapabilityNotification` | no |
| **removed** `query.View(...).RelationalSource(loader)` | no |
| **removed** `read.RootScanner` / `read.ChildScanner`, `WithRootScanner` / `WithChildScanner` | no |
| `mongo.CheckServiceRegistry` gains `upstreamCollections []string` | no |

Because no item lands on this service, the per-item old-pin/new-pin documentation
reconciliation the gate normally performs had no subject. No contract in this project was
migrated, because none was in use.

## Operational fallout — five classes, each checked

| Class | Finding for `authcore` |
|---|---|
| (a) required DDL on the service's own tables | **none demanded**, and none possible — no entity tables exist yet |
| (b) demanded view rebuild | **none** — no read model is declared |
| (c) framework's embedded migration sequence grew | **no** — the embedded set is byte-identical across the two pins (`0001_framework`, `0002_view_slots`, `0003_projection_failures`, per engine). No pending-migration boot abort from this bump under `autoRun: check` |
| (d) yaml key renames / moves | **none** — the 307-line per-symbol `CHANGELOG.md` block for v0.57.0 names no configuration key; `microservice.dev.yaml` / `microservice.prd.yaml` are untouched |
| (e) shared gRPC proto contract changed | **no** — the `.proto` file set and every file's content are identical between v0.56.1 and v0.57.0, and `authcore` ships no `.proto` / `.pb.go` of its own. No regeneration step |

## Needs your attention — not auto-fixable

Nothing here blocks the upgrade. These are v0.57.0 semantics that will apply the moment
this service grows the surfaces they govern.

1. **A read-model name may not end in `__0` or `__1`.** Those are the blue-green slot
   suffixes, refused at boot in every read-model family (`query.View`, `SharedBaseView`,
   `ComposedView`, `RelationalView`) — one namespace. Relevant when the first view is
   scaffolded, not before.
2. **Read-side behavior moved under the fixes**, on surfaces this service does not yet
   expose: keyset pagination no longer stalls when the selection omits the id;
   `ReadCriteria.Restrict` now withholds the identity on Mongo-backed views too;
   a field-restricted listing can be sorted again (`?orderBy=` used to 500);
   a composed view no longer serves an unrequested id; and an unresolvable `?fields=`
   path on a relational view is now `SchemaViolationNotification` /
   `SemanticSchema` → **400** rather than a silent `200 {}`. No consumer of `authcore`
   can have depended on any of this — there is no endpoint yet.
3. **An `upstreamSubscriptions` mirror collection is no longer reported foreign** by the
   DB-per-service guard. Before v0.57.0 a service declaring an upstream subscription could
   not boot outside `dev`. `authcore` declares none, so this is headroom, not a fix applied.
4. **New capability worth knowing about before the first entity lands:** read joins
   (`docs/content/sections/read-joins.html`) let an aggregate's queries filter, sort and
   return a few columns from another aggregate across a foreign key, declared once on the
   repository via `WithJoins` and inherited by every reader; and `query.RelationalView`
   (`docs/content/sections/relational-view.html`) serves a read model straight from the
   relational store — no version, no registry row, no rebuild, no Mongo collection. Both
   are relevant to how `authcore`'s first entities and read models get shaped.
