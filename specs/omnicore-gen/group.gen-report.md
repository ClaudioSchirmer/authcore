# Group — generation report

Generated from `specs/omnicore-gen/group.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you write

Declared as `kind: manual` (a scalar whose rule is beyond this language) or as a composite with `written: manual` (its shape is declared, its file is yours), so the generator wrote NO file for them — the emitted code already declares fields of these types and converts to and from them, so the package does not compile until each one exists:

- **`GroupKey`** — `internal/domain/vos/group_key.go`. A group's stable machine handle: a lowercase slug of 2-64 runes, groups separated by single hyphens so a hyphen can never lead, trail or double. Reuses this project's shared anti-junk predicates. No reserved list and no derivation — those two rules belong to the tenant handle. No normalization: a value that does not already comply is refused, never repaired.
  ```go
  type GroupKey string
  func (v GroupKey) Value() string { return string(v) }
  func (v GroupKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool
  ```
  The underlying type is `string` and is not negotiable: the mappers convert with `vos.GroupKey(x)` and read back with `.Value()`. `IsValid` is the framework's entry point — it is found by TYPE, with no registration, and reports every problem it finds through the context rather than returning one, so a caller sees all of them at once.

### `internal/domain/group_rules_manual.go`

The spec declared these invariants as ones it could not express. The file was just created, with a stub for each; the code is yours to write, and regeneration will never touch it.

**`tenant-must-be-available`**

> The owning tenant must exist, must not be archived, and must not be SUSPENDED. Ask TenantIsUnavailable. A trial tenant is a live customer and passes — "unavailable" is not "not active". A read join cannot answer this: on an insert a joined field is blank, because nothing traversed a foreign key for a struct the mapper built from a request body a millisecond ago.

- fires under `IfInsert` · raise `GroupTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`attached-roles-are-available-in-this-tenant`**

> Every role this write ATTACHES must exist, still be active, AND belong to THIS GROUP'S TENANT. Walk the ADDED entries only (GetAddedItemsOf, not GetCurrentItemsOf) and ask RoleIsUnavailableInTenant per entry, passing the group's own TenantID. The same-tenant half is the first thing in this entity that Role did not need: Permission is a global catalog, Role is tenant-scoped, so a group in tenant A attaching tenant B's role would confer another customer's permissions on A's members. Do NOT read the entry's RoleKey/RoleName join fields to decide this: an added entry carries "" in both, which is indistinguishable from a role whose key is genuinely empty — a security rule that passes on a blank field is the worst possible failure direction. Judging only ADDED entries is deliberate: a role archived AFTER it was attached must not make the group impossible to rename, and a detach must stay reachable.

- fires under `IfInsertOrUpdate` · raise `RoleNotAvailableInTenantNotification{}` · attach it to `Roles`

**`no-wildcard-role-attach`**

> A role granting a permission with a wildcard in either part cannot be attached to any group. Runs AFTER the availability rule and BEFORE the escalation rule, and both halves of that order are load-bearing — see the block comment above. Over the ADDED entries only; ask RoleGrantsWildcard per entry, which answers TRUE for an unresolvable role id so an unknown id never falls through to the escalation question. What it costs, stated plainly: the platform's own superadmin group cannot be created through this API, and is seeded by migration beside the reserved platform tenant and the *:* role.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardRoleNotification{}` · attach it to `Roles`

**`no-privilege-escalation`**

> TRANSITIVE no-escalation: a caller may attach a role only if they hold EVERY permission that role grants — a set, not one key. Over the ADDED entries only; ask CallerLacksAnyPermissionOf per entry, which runs after the wildcard refusal so every key reaching it is concrete. The *:* superadmin exemption is free: HasPermission returns true for any concrete permission when the claim set contains *:*. Two states, not one — ctx.Identity() nil (auth disabled, dev only, and the framework's own boot guard refuses that profile anywhere else) means NO identity gate; an identity present with an insufficient claim REFUSES. Collapsing them makes the entity unusable in dev.

- fires under `IfInsertOrUpdate` · raise `CannotGrantRoleWithUnheldPermissionsNotification{}` · attach it to `Roles`

Its tests are yours too — the generator does not know what these rules mean.

### `internal/infra/group_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owning tenant is missing, archived, or commercially SUSPENDED. Queries the tenants table by its primary key. A trial tenant is a live customer and is available.

**`RoleIsUnavailableInTenant(tenantID domain.ID, roleID domain.ID) bool`**

> Whether this role id is absent from the roles table, points at an archived role, or belongs to a tenant OTHER than the one passed. One answer for all three questions — the caller-facing message must not distinguish them, because a distinct "belongs to another tenant" reply is an existence oracle over a competitor's org chart.

**`RoleGrantsWildcard(roleID domain.ID) bool`**

> Whether the role behind this id grants any permission carrying a wildcard in either part. Resolve the role through RoleRepository.Loader, whose declared read join fills every grant's resource and action. Answers TRUE when the id is unknown, so an unresolvable attachment never reaches the escalation probe.

**`CallerLacksAnyPermissionOf(roleID domain.ID) bool`**

> Whether the requesting caller fails to hold at least one of the permissions this role grants — the TRANSITIVE half of the escalation rule. Resolves the role through the same single read RoleGrantsWildcard uses, then asks the caller's claim for every key it found. Judges EVERY key the role grants, archived catalog rows included: that is the fail-closed direction, and since the grants no longer carry the catalog row's archive stamp it is also the only reading expressible without a second query. It GUARDS THE WILDCARD ITSELF and answers "lacks" rather than calling through — defence in depth behind the wildcard rule, because a panic on a security rule is a 500. Reads ctx.Identity() through the request-scoped service.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

### Rules that END the validation pass

`tenant-is-a-usable-id` (in `IfInsertOrUpdate`) — declared `guard: true`. After each of them the pass stops if anything has already been rejected: the rules below it, this entity's automatic value-object validation, and the `BuildRules` and value objects of every collection. A clean write is unaffected — the barrier fires only where something was ALREADY refused — so what changes is the SHAPE of a 422: it carries what was found up to the barrier, not that plus every other field the write would have failed on. Check that against what the API consumers expect to receive in one response.

### Value objects validated with the rules, not after them

`TenantID` (in `IfInsertOrUpdate`) — declared `kind: valueObject`. The framework validates every value-object field on every write, but that pass runs AFTER `BuildRules`, so a value object cannot be the premise of the rules below it. Each field above is checked where it is declared instead — by the value object's own answer, never a second one — and excluded from the automatic pass in those verbs, so nothing is reported twice. Two consequences worth checking: the value is now validated BEFORE any barrier that follows, so a 422 that used to hide it behind another failure now carries both; and in the verbs this rule does not cover, the field is still validated at the end, exactly as before.

### Per-entry command tests are generated now

The verbs that address ONE entry — add, remove — have generated tests in `internal/application/commands/group_commands_test.go`: the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

**If you wrote your own tests for those mappers before this run**, the package will not compile until you delete them — Go reports it as `redeclared in this block`, which reads like a generator bug and is not one. The generated cases cover the same ground; anything yours asserts beyond them is worth keeping under a different name.

## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `groups` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Collection `Roles` | `add` → `group:grant` (declared); `remove` → `group:grant` (declared) | These routes hang off `/groups/:id/roles`. Gated on its own through `children[].permissions`, not by the root's update. Grant that permission before the routes go live — a holder of the root's update alone now gets a 403 here. |
| Removal | archive (reversible) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Key` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `GroupKeyAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. Nothing is materialised: there is no collection, no version and no rebuild — a shape change here needs no bump and no operational step. |
| Read join → Tenant | `InnerJoin` on `tenant_id` | An aggregate with no counterpart is NOT returned, on EVERY read through this repository — FindByID included, which the write handlers load through. Legal only because the foreign key is non-nullable. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |
| Read join → Role | `InnerJoin` on `role_id`, from GroupRole | An entry with no counterpart is NOT returned — a silent hole in the collection, not a missing aggregate. Prefer left wherever the relationship is genuinely optional. Nothing here is a write path: the fields are absent from the TableSchema, so no INSERT or UPDATE can carry them and no migration creates them. |

## What was generated

| What | File |
|---|---|
| the groups feature (repository + view + mount) | `bootstrap/groups_feature.go` |
| the GroupsFeature registration in the composition root | `bootstrap/wire.go` |
| the archive command and result | `internal/application/commands/archive_group_command.go` |
| the shapes for 1 child collection(s) | `internal/application/commands/group_child_results.go` |
| tests for the command mappers | `internal/application/commands/group_commands_test.go` |
| the per-entry commands for group_roles | `internal/application/commands/group_role_commands.go` |
| the insert command and result | `internal/application/commands/insert_group_command.go` |
| the patch command and result | `internal/application/commands/patch_group_command.go` |
| tests for the 1 collection input mapper(s) | `internal/application/dtos/group_dtos_test.go` |
| the GroupRole input DTO | `internal/application/dtos/group_role_input.go` |
| the by-id query and its result | `internal/application/queries/find_group_by_id_query.go` |
| the listing query and its result | `internal/application/queries/find_groups_by_params_query.go` |
| the read criteria tests | `internal/application/queries/group_queries_test.go` |
| the read shapes for 1 child collection(s) | `internal/application/queries/group_row_results.go` |
| 22 DEU translation key(s) | `internal/application/translations/deu.go` |
| 22 ENG translation key(s) | `internal/application/translations/eng.go` |
| 22 ESP translation key(s) | `internal/application/translations/esp.go` |
| 22 FRA translation key(s) | `internal/application/translations/fra.go` |
| the translation coverage test — every notification must be translatable in every catalog | `internal/application/translations/group_translations_test.go` |
| 22 ITA translation key(s) | `internal/application/translations/ita.go` |
| 22 NLD translation key(s) | `internal/application/translations/nld.go` |
| 22 PTBR translation key(s) | `internal/application/translations/ptbr.go` |
| tests for the collection types | `internal/domain/aggregatevos/group_children_test.go` |
| the GroupRole child value object | `internal/domain/aggregatevos/group_role.go` |
| the Group aggregate root, its modes and its rules | `internal/domain/group.go` |
| the hand-written rules for Group (4 to implement) | `internal/domain/group_rules_manual.go` |
| the Group service port (5 fact(s)) | `internal/domain/group_service.go` |
| tests for Group's rules | `internal/domain/group_test.go` |
| 9 notification declaration(s) | `internal/domain/notifications.go` |
| 1 notification declaration(s) | `internal/domain/vos/notifications.go` |
| the Group repository and its constraint bindings | `internal/infra/group_repository.go` |
| the Group service implementation | `internal/infra/group_service.go` |
| the hand-written facts for Group (4 to implement) | `internal/infra/group_service_manual.go` |
| the group_roles child schema | `internal/infra/schemas/group_role_schema.go` |
| the groups schema (4 columns) | `internal/infra/schemas/group_schema.go` |
| the schema builder tests — they run the builders, so a boot panic is a test failure | `internal/infra/schemas/group_schemas_test.go` |
| the groups view (relational-backed) | `internal/infra/views/group_view.go` |
| the view definition test — it builds the definition, so a boot panic is a test failure | `internal/infra/views/group_view_test.go` |
| the 5 group endpoints | `internal/web/group_routes.go` |
| the by-id request and response | `internal/web/requests/find_group_by_id.go` |
| the listing request and response | `internal/web/requests/find_groups_by_params.go` |
| the wire types for 1 child collection(s) | `internal/web/requests/group_children.go` |
| the request mapper tests | `internal/web/requests/group_requests_test.go` |
| the per-entry wire types for group_roles | `internal/web/requests/group_role_requests.go` |
| the insert request and response | `internal/web/requests/insert_group.go` |
| the patch request and response | `internal/web/requests/patch_group.go` |
| the rollback of groups on postgres | `migrations/postgres/0004_group_manual.down.sql` |
| the groups table on postgres | `migrations/postgres/0004_group_manual.up.sql` |

1 file(s) were already up to date.

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.59.0)

framework v0.59.0 meets the required v0.59.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
