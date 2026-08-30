# Migration plan — omnicore v0.54.0 → v0.55.0

Status: APPROVED

Decision on the open item: `?orderBy=` gets an explicit vocabulary rather than staying
open — **`sort: [Name, Workspace, CreatedAt]`**, which is what the service ships and what
`../scaffold-entity/tenant/spec.md` §9 records. Two of the three are not index-backed;
§1 says which and what that costs.

Service: `authcore` · Build tags: `postgres` (engine from `relational.dialect`; neither
profile declares a `transport:` block)

The bump is already applied — `go.mod`/`go.sum` are on v0.55.0 and
`go vet -tags postgres ./...` + `go build -tags postgres ./...` are both **green**.
Nothing in this project imported the five removed `web/queryschema` symbols,
`TableSchema.FieldResolver`, `ColumnForRead`, `MountRaw`/`CriteriaBuilder`, `TextIndex`
or `query:"search"`, so v0.55.0's compile-visible breaks miss this service entirely.

**The fallout here is not compile-visible.** It is one boot-blocking item, plus
behavioral changes on the wire.

Rollback point: `specs/upgrade/rollback/` (verbatim pre-bump `go.mod` + `go.sum`;
both files are also git-tracked and were clean at snapshot time).

---

## 1. ⛔ BOOT BLOCKER — `?orderBy=` has a switch and no vocabulary

### The finding

No compile error. The guard fires at boot.

`internal/web/requests/find_tenants_by_params.go:45` declares the switch:

```go
OrderBy *string `query:"orderBy"`
```

and **no leaf anywhere in the project carries a `sort:` tag** (verified:
`grep -rn 'sort:"' --include='*.go' .` → no matches).

This is reproduced independently by the generator's own check. Running
`omnicore-gen check` against the *current, unmodified* spec on the v0.55.0 pin
(executed on a throwaway clone, the project untouched) answers:

```
1 blocker(s):
  ✗ read.byParams.sort: controls.orderBy is the SWITCH for ?orderBy= and nothing
    declares the vocabulary it switches on — the endpoint would accept the parameter
    and then refuse every token it could be given
      → list the orderable paths under sort: — nothing is orderable until it is named,
        because an unindexed sort is a blocking sort whose cost grows with the
        matching set
```

### How it worked at v0.54.0

`docs/content/sections/auto-query-handlers.html`, *"Sortable response paths via `?orderBy=`"*:

> The Request DTO opts in by declaring `OrderBy *string \`query:"orderBy"\`` […]

The switch stood alone. The **vocabulary came from the Response projection schema** —
every path `FindTenantsResponse` rendered was sortable, and the only boot-time reaction
was a `slog.Warn` listing the sortable wire paths so the maintainer could check index
coverage. For this service that meant `id`, `tenantID`, `name`, `workspace`,
`description`, `status` were all orderable, in both directions, for free.

### How it works at v0.55.0

Same file, new section *"The ordering pair — `query:"orderBy"` and `sort:`"*:

> Ordering is a store operation, like filtering. Its vocabulary therefore lives where the
> endpoint declares what it accepts on the wire — the Request DTO — and never on the
> Response […] It is declared in two halves, which answer different questions and must
> travel together […] Half a declaration is a boot failure, and each half has its own
> diagnostic. The switch with no vocabulary would accept `?orderBy=` and then refuse
> every token it could be given […] **Nothing is orderable until a leaf says so.**

The grammar is closed: each `sort:` entry is `asc` or `desc`, listed at most once;
anything else — including an empty tag — is a boot panic.

### The proposed edit

`find_tenants_by_params.go` is **omnicore-gen generated with a checksum guard**, so a
hand edit is refused by the next generator run. The fix belongs in the spec.

**File: `specs/omnicore-gen/tenant.omnicore.yaml`** — add a `sort:` list under
`read.byParams`, immediately above `controls:`:

```yaml
    sort: [Name, Workspace, CreatedAt]
    controls:
      pagination: true
      orderBy: true
      ...
```

Then regenerate: `omnicore-gen generate`.

The generator emits `sort:"asc,desc"` on each named leaf:

```go
Name      *string    `query:"name" filter:"eq,in,startswith,contains,istartswith,icontains" sort:"asc,desc"`
Workspace *string    `query:"workspace" filter:"eq,in,startswith,istartswith" sort:"asc,desc"`
CreatedAt *time.Time `query:"createdAt" filter:"eq,gte,lte,gt,lt" sort:"asc,desc"`
```

`TenantID`, `Description`, `Status` and `UpdatedAt` keep their `filter:` tags and gain no
`sort:` tag: still filterable, not orderable.

The spec's `sort:` is a plain list of field names; the generator always emits both
directions. Per-direction control (`sort:"asc"` only) is not expressible through this
spec key.

### RESOLVED: which paths are orderable

`sort: [Name, Workspace, CreatedAt]` — the vocabulary this service ships, and what
`specs/scaffold-entity/tenant/spec.md` §9 records. `?orderBy=` is a two-half declaration:
this list is the vocabulary, `controls.orderBy` is the switch, and either half alone fails
the boot.

Paths that were orderable at v0.54.0 and are not any more:

| Path | Was | Now | Why dropped |
|---|---|---|---|
| `description` | orderable | **400** | Long free text; ordering a listing by a 500-character column is not a meaningful sequence. |
| `status` | orderable | **400** | Low cardinality — a filter serves this better, and `status` is already filterable `eq,in`. |
| `id` | orderable | **400** | Not in the vocabulary. It IS declarable — `sort: [ID]` is an ordinary entry and the id is the one path indexed before anybody asks — so this is the model's choice, not a limit. The keyset cursor appends the id as its trailing tiebreak either way, so the listing stays deterministic without it. |
| `tenantID` | orderable | **400** | Not in the vocabulary. Ordering by an opaque derived UUID is ordering by nothing a reader can follow; it remains filterable `eq,in`, which is how a consuming service resolves a token claim back to a tenant. |

**The shipped vocabulary is NOT indexed-only, and that is the one thing to be deliberate
about here.** Of the three paths admitted, only `workspace` is index-backed:

| Path | Index in `migrations/postgres/0001_tenant_manual.up.sql` |
|---|---|
| `workspace` | `CREATE UNIQUE INDEX tenants_workspace_key ON tenants (workspace)` |
| `name` | **none** — a blocking sort whose cost grows with the row count |
| `createdAt` | **none** — same |

Option (C), "indexed only", was the resolution recorded when this plan was written and it
is not what the service ships: `name` and `createdAt` are admitted without an index. That
is defensible while `tenants` is small — this table grows with the customer base, not with
traffic — and it is a real cost to revisit when it is not. Closing it is a new numbered
migration adding a `tenants (name)` and a `tenants (created_at)` index; narrowing the
vocabulary instead is one line in the spec and a regeneration. Either way it is a decision
someone should take on purpose rather than inherit.

**Consumer impact to communicate:** `?orderBy=description`, `?orderBy=status`,
`?orderBy=id` and `?orderBy=tenantId` (and their `-` descending forms) answer 400
`SchemaViolationNotification` on `orderBy[<token>]`. Reinstating any of them means adding
it to `sort:` and regenerating — plus an index, if the intent is to keep every admitted
sort backed by one.

### Knock-on: the GraphQL schema narrows

`surfaces.graphql.connection: true`. GraphQL derives its `<Entity>OrderField` enum from the
same `sort:` set and cuts direction in the resolver (an enum cannot express per-member
directions), so the `TenantOrderField` members are exactly `NAME`, `WORKSPACE` and
`CREATED_AT` — a **breaking GraphQL schema change** for any client naming a member that is
gone. Regenerate/redistribute the schema to consumers alongside this deploy.

---

## 2. Boolean read controls are now strictly parsed — report only, no edit

No edit is proposed. This is a wire-behavior change on endpoints this service already
serves, and it is worth knowing before it reaches a consumer.

Declared here:

- `internal/web/requests/find_tenants_by_params.go:47` — `OnlyTotal *bool \`query:"onlyTotal"\``
- `internal/web/requests/find_tenants_by_params.go:48` — `IncludeArchived *bool \`query:"includeArchived"\``
- `internal/web/requests/find_tenant_by_id.go:34` — `IncludeArchived *bool \`query:"includeArchived"\``

At v0.54.0 the two REST paths disagreed: the paged wrapper compared strings and never
failed, while the by-id wrapper delegated to Fiber's binder and accepted `1`, `t`,
`TRUE`. So `?includeArchived=1` was **false** on the listing and **true** on a by-id
read of the same entity.

At v0.55.0 both accept exactly `true` or `false`; everything else is the canonical 400.
Presence is the key being on the query string, not the value being non-empty — so a bare
`?includeArchived=` is now refused on both routes.

**Action for you:** if any client, script, or QA case sends `?includeArchived=1` (or `t`,
`TRUE`, or bare), it now gets a 400. Grep your callers. Note this service declares
`includeArchived` deliberately — per the spec comment, *"Archived tenants must stay
reachable: archiving is the only removal there is"* — so the control is load-bearing here.

---

## 3. Regeneration side effects — cosmetic, listed so nothing is a surprise

Running `omnicore-gen generate` (generator 0.25.0, which requires framework v0.55.0+;
the tree was last generated by 0.23.0) rewrites more than the DTO:

- `specs/omnicore-gen/lock.json` — records framework v0.55.0 instead of v0.54.0.
- `specs/omnicore-gen/tenant.gen-report.md` — regenerated report.
- `internal/domain/vos/tenant_vos_test.go` — **header only.** The comment
  `// Tests for 4 value object(s).` becomes `// Tests for 1 value object(s).`, plus a new
  date and checksum. Verified: same line count (67), identical set of `func Test…`
  declarations. A generator header-counting difference between 0.23.0 and 0.25.0,
  carrying no test-content change.

Three files are `kept as-is (yours, by design)` and are not touched.

---

## 4. Operational classes — checked, all clear

The five classes a version bump can demand outside Go code, each verified against the
two module-cache trees rather than assumed:

| Class | Verdict | Evidence |
|---|---|---|
| (a) Required DDL on this service's own tables | **None** | No such mandate in the v0.55.0 changelog; no managed-column addition announced. |
| (b) Demanded view rebuild | **None** | `docs/content/sections/views.html` byte-identical across the two pins. The read change is a request-vocabulary change, not a projected-shape change — so `TenantView`'s `Version(1)` stays as it is. |
| (c) Framework's embedded migration sequence grew | **No** | `diff` of every `*migration*/*.sql` path across both pins → no delta (30 files, identical). `docs/content/sections/migrations.html` byte-identical. No `autoRun: check` boot abort to expect. |
| (d) yaml key renames / moves | **None** | `docs/content/sections/yaml-reference.html` byte-identical across pins. `microservice.dev.yaml` / `microservice.prd.yaml` need no edit. |
| (e) Shared gRPC proto contract changed | **N/A** | This service ships no `.proto` and no `.pb.go`; no gRPC surface is mounted. |

---

## 5. Needs your attention — behavioral, not auto-fixable

A green build proves none of these. Each is a v0.55.0 runtime change that lands on a
surface this service exposes.

1. **GraphQL by-id now resolves its selection set.** Previously it discarded it, so
   nothing became a projection there and `ReadCriteria.Restrict` — the field-level
   access-control seam — did not apply at all on the singular field. A field restricted
   through `tenants(...)` was **scrubbed in silence** through `tenant(id:)`. It now
   answers 403 to an active reference, like the connection field. `read.fieldRestrict`
   is empty in this spec today, so nothing changes *now* — but the semantics you would
   get from adding one have changed.
2. **By-id reads honor `ReadCriteria.Projection` on Mongo.** This view is
   `backing: relational`, so the relational reader already applied it; noted for
   completeness in case the backing is ever flipped.
3. **GraphQL and gRPC now run the cursor structure check REST always ran.** A cursor
   GraphQL used to accept and REST rejected is now rejected on both.
4. **A by-id read refuses an undeclared control on every surface.** Previously the by-id
   procedure quietly ignored a control its REST twin refused.
5. **`ComputedFieldNotSortableNotification` is removed.** No computed fields
   (`computed:"…"`) exist in this spec, so nothing here depends on it.
6. **`?fields=` now self-documents in OpenAPI** — the generated document gains a rule
   statement and a dotted example. Cosmetic, but the OpenAPI output changes.

None of the above is proposed as an edit. They are the reasons a green build is not the
same as unchanged behavior.

---

## Verification after applying

```
omnicore-gen generate
go vet -tags postgres ./...
go build -tags postgres ./...
go test -tags postgres ./... -count=1
```

Then boot the service — the ordering guard is a boot check, so a successful start is the
real proof this plan worked.
