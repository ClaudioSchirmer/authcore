# Migration plan — omnicore v0.67.1 → v0.68.0

Status: APPROVED

Decision on the open item (§4): **(a) regenerate**. The five generated repositories are
rewritten whole against the v0.68.0 emitter; the four declarations in
`authentication_reader_manual.go` are edited by hand, as §2 states.

## Why a green build proves nothing here

`go vet -tags postgres ./...` and `go build -tags postgres ./...` both pass on v0.68.0,
and so does the offline unit suite. That is not evidence.

The join constructors did NOT change shape between the two pins:

    v0.67.1  infra/db/command/read/join.go:122  func InnerJoin(target *core.TableSchema) *JoinBinding
    v0.68.0  infra/db/command/read/join.go:122  func InnerJoin(target *core.TableSchema) *JoinBinding

The new requirement is enforced at runtime, inside `WithJoins(...)`:

    v0.68.0  infra/db/command/read/join.go:482
        if !j.Target.IsDirect() {
            failAt(path, "%s(%q): the target of a read join is ONE table, so it takes a DIRECT schema — ...")
        }
    v0.68.0  infra/db/command/read/join.go:417
        panic(fmt.Sprintf("read.WithJoins[%s]: ", contextName) + msg)

`validateJoins` is reached from `directCore.declareJoins` (`direct_core.go:118`), which
both `DirectRepository.WithJoins` and `AggregateLoader.WithJoins` funnel through — so
every declaration in this service is covered, root joins and child joins alike
(`join.go:549-562`: the per-join loop calls the same `walk` for both).

`IsDirect()` is `s.direct` (`core/table_schema.go:288`), a flag set only by
`NewDirectSchema` and `AsDirectSchema`. Every target this service joins to is built with
`core.NewTableSchema[...]`:

    internal/infra/schemas/tenant_schema.go:44      core.NewTableSchema[*appdomain.Tenant]("tenants")
    internal/infra/schemas/role_schema.go:44        core.NewTableSchema[*appdomain.Role]("roles")
    internal/infra/schemas/group_schema.go:44       core.NewTableSchema[*appdomain.Group]("groups")
    internal/infra/schemas/claim_schema.go:44       core.NewTableSchema[*appdomain.Claim]("claims")
    internal/infra/schemas/permission_schema.go:45  core.NewTableSchema[*appdomain.Permission]("permissions")

So all 16 declarations panic the moment their repository is constructed — which is
bootstrap. The unit suite stayed green because the only test that constructs one
(`internal/infra/authentication_reader_live_test.go:70`) sits behind
`//go:build integration && postgres`.

## How it worked at v0.67.1 / how it works at v0.68.0

Owning section: `docs/content/sections/read-joins.html`, read at both pins.

**v0.67.1** — a traversal took any `*core.TableSchema`. The docs' own examples read
`read.InnerJoin(schemas.CustomerSchema()).On("customer_id")` (line 17). A schema carrying
children, siblings or a shared base entered whole and was read in part, silently. Worse,
`validateJoins` resolved the mapped column through `GoNameForRead`, which merges the
target's satellites — so `.Field("Doc", "documento")` naming a column of the target's
SIBLING was accepted at boot and then emitted as `alias.documento` against the target's
own table: a SQL error on every read through that loader, `FindByID` included.

**v0.68.0** — new subsection *"The target is one table — a Direct schema"* (lines 46-68):

> A traversal puts the target in the FROM as one table under one alias. So it takes a
> schema that is one table: a Direct schema. A schema carrying children, siblings or a
> shared base is refused at the declaration [...] Any schema becomes a target, at the call
> site, where the reduction is visible.

The docs' examples move to `read.InnerJoin(schemas.CustomerSchema().AsDirectSchema())`.
`AsDirectSchema()` (`core/table_schema.go:1473`) returns a COPY reduced to the schema's
own table; the receiver is untouched. An aggregate root, a child, a role, a shared base
and a Direct schema all convert — a sibling and an external schema panic instead. Nothing
reachable before is out of reach now.

## 1. The five generated repositories — 12 declarations

    internal/infra/user_repository.go:77    read.InnerJoin(schemas.TenantSchema())
    internal/infra/user_repository.go:83    ...InChild(schemas.UserGroupSchema()).To(schemas.GroupSchema())
    internal/infra/user_repository.go:89    ...InChild(schemas.UserRoleSchema()).To(schemas.RoleSchema())
    internal/infra/user_repository.go:95    ...InChild(schemas.UserClaimSchema()).To(schemas.ClaimSchema())
    internal/infra/client_repository.go:77  read.InnerJoin(schemas.TenantSchema())
    internal/infra/client_repository.go:83  ...InChild(schemas.ClientRoleSchema()).To(schemas.RoleSchema())
    internal/infra/client_repository.go:89  ...InChild(schemas.ClientClaimSchema()).To(schemas.ClaimSchema())
    internal/infra/group_repository.go:75   read.InnerJoin(schemas.TenantSchema())
    internal/infra/group_repository.go:81   ...InChild(schemas.GroupRoleSchema()).To(schemas.RoleSchema())
    internal/infra/role_repository.go:75    read.InnerJoin(schemas.TenantSchema())
    internal/infra/role_repository.go:81    ...InChild(schemas.RolePermissionSchema()).To(schemas.PermissionSchema())
    internal/infra/claim_repository.go:74   read.InnerJoin(schemas.TenantSchema())

Only the TARGET takes the reduction. The child named by `...InChild(...)` must stay the
ordinary child schema: `validateJoins` matches it against `root.ChildSchemas()` by table
name (`join.go:424-426`, `join.go:551-557`), and it is not a FROM entry.

All five files carry `// Code generated by omnicore-gen. DO NOT EDIT.` and `doctor`
reports no adopted hand edits on any of them, so both paths below are open.
See §4 — this is the plan's one open decision.

## 2. `internal/infra/authentication_reader_manual.go` — 4 declarations

Hand-written; no generator will fix these whichever path §4 takes.

    :90   read.InnerJoin(schemas.RoleSchema()).On("role_id")        → .RoleSchema().AsDirectSchema()
    :96   read.InnerJoin(schemas.GroupSchema()).On("group_id")      → .GroupSchema().AsDirectSchema()
    :101  read.InnerJoin(schemas.RoleSchema()).On("role_id")        → .RoleSchema().AsDirectSchema()
    :107  read.InnerJoin(schemas.PermissionSchema()).On("permission_id") → .PermissionSchema().AsDirectSchema()

Proposed edit: insert `.AsDirectSchema()` on each of the four targets. Nothing else in the
file changes — the `.On(...)`, `.Field(...)` and archive predicates are untouched, and the
anchors (`UserRoleEdgeSchema`, `GroupRoleEdgeSchema`, `UserGroupEdgeSchema`,
`RolePermissionEdgeSchema`) are already Direct schemas built by
`internal/infra/schemas/grant_edge_direct_schemas.go`.

Worth noting in passing: the fields these four joins map are all columns of the target's
OWN table (`role_key`, `name`, `resource_name`, `action_name`, `deleted_at`), so this
service was never hitting the silent-satellite bug the release fixes. The change here is
conformance, not a latent defect being closed.

## 3. `criteria.Sub` — nothing to do

The v0.68.0 subquery API (`Sub`, `Outer`, `InSub`/`NinSub`/`EqSub`/…/`Exists`/`NotExists`)
is new surface. `grep -rn 'criteria\.Sub' --include='*.go'` returns nothing in this
service, so the matching Direct-schema requirement on a subquery source
(`core/criteria_sql.go:473`) has no call site to migrate.

It is worth a separate look later, not in this plan: `AuthenticationReader`'s grant walk
reads four edge tables in sequence, which is the exact shape
`Exists(Sub(...).Where(Eq(..., Outer("ID"))))` now expresses in one statement. That is a
design change with its own trade-offs (one snapshot vs. four, and the per-hop archive
predicates the reader states by hand), not an upgrade fix.

## 4. ⚠️ OPEN: how to fix the 12 generated declarations

Both paths end at the same compiling, booting service. They differ in blast radius and in
what else they carry.

**(a) Regenerate.** `omnicore-gen check` reports `✓ this spec can be generated` for all
seven specs at `Framework: v0.68.0 (supported v0.68.0.x) — exact`. The lock records the
specs at v0.63.0/v0.64.0, so regeneration rewrites the five files whole against the 0.68
emitter — the join fix arrives with every other emitter change accumulated across five
releases, which is the point of not having adopted files. The cost is that the diff to
review is five whole files, not twelve lines, and any of it that is not the join fix is
unreviewed change landing in the same commit.

**(b) Hand-edit the twelve lines.** Insert `.AsDirectSchema()` on each target, exactly as
§2 does for the manual reader. Deterministic, twelve-line diff, nothing else moves. The
files keep their `DO NOT EDIT` banner and drift from the emitter — a later `generate`
would overwrite the edit, which is fine here because the emitter would then write the same
thing. The cost is that the five releases' worth of other emitter improvements stay
unclaimed until the next regeneration.

Recommendation: **(a)**, and review the resulting diff before anything else — the whole
reason `doctor` reports no adopted files is that regeneration is supposed to be cheap
here. If the diff turns out to carry more than expected, (b) stays available.

**ANSWERED: (a) regenerate.**

## 5. Not auto-fixable — for your attention

- **Nothing operational.** No new mandatory yaml key in v0.68.0 (`relational.clock` was
  the v0.65.0 arrival and is already set in both profiles). The framework's embedded
  migration set is byte-identical between the two pins. No DDL on this service's tables,
  no view rebuild demanded, no gRPC proto change.
- **The real verification is the boot, not the build.** Once the plan is applied, the
  proof is `/omnicore:run` — or the integration suite with
  `go test -tags 'integration postgres' ./internal/infra/...` against the dev bench, which
  is the only offline path that actually constructs `AuthenticationReader`.
- **Upstream doc bug, for the framework repo — not this service.** The v0.68.0
  `read-joins.html` section *"Joining from an aggregate child"* (line 329) still shows
  `read.LeftJoinInChild(schemas.EnderecoSchema()).To(schemas.CidadeSchema())` with no
  `.AsDirectSchema()` on the target. The code refuses that form: the child-join loop calls
  the same `walk`, so `j.Target.IsDirect()` applies there too. Every other example in the
  file was updated. The example is stale, not a second contract.

## Applied — 2026-08-31

**§1 (regenerate)** — `omnicore-gen generate` on all seven specs. Seven files updated,
nothing created, `kept as-is` untouched. The diff is exactly the twelve join sites plus a
comment explaining the reduction, the regenerated checksum/date header, and one reworded
comment line in `permission_service.go` and `tenant_service.go` ("counting them in Go" →
"folding the answer in Go"). No behavioral change outside the joins. The emitter writes
`To(schemas.RoleSchema().AsDirectSchema())` for child joins, which confirms §5's reading
that the docs' `Joining from an aggregate child` example is stale rather than a second
contract.

**§2 (by hand)** — the four targets in `authentication_reader_manual.go` (:90, :96, :101,
:107) now carry `.AsDirectSchema()`. Nothing else in the file changed.

`grep -rnE 'read\.(InnerJoin|LeftJoin)\([^)]*Schema\(\)\)|\.To\(schemas\.[A-Za-z]+Schema\(\)\)'`
returns nothing: no unreduced target is left.

**Verify:** `gofmt -l internal/` clean · `go vet -tags postgres ./...` clean ·
`go build -tags postgres ./...` clean · `go test -tags postgres ./...` all packages ok ·
`go vet -tags 'integration postgres' ./internal/infra/...` clean (the integration suite
compiles).

**Still unproven, by construction.** None of the above constructs a repository, so none of
it exercises `validateJoins`. No Postgres bench is reachable from this run
(`DATABASE_URL` unset, no container up), so the boot was not performed. The proof is
`/omnicore:run`, or `go test -tags 'integration postgres' ./internal/infra/...` against a
live bench.
