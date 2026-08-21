# Role — generation report

Generated from `specs/omnicore-gen/role.omnicore.yaml`.

The descriptions, examples and labels quoted here are in **en-US**, as the spec declares.

## What still needs implementing

### Value objects you write

Declared as `kind: manual` (a scalar whose rule is beyond this language) or as a composite with `written: manual` (its shape is declared, its file is yours), so the generator wrote NO file for them — the emitted code already declares fields of these types and converts to and from them, so the package does not compile until each one exists:

- **`RoleKey`** — `internal/domain/vos/role_key.go`. The stable machine handle of a role: 2-64 runes, a single lowercase slug of letters, digits and single hyphens, never leading, trailing or doubled. It carries no reserved list and no derivation — it is a handle within one tenant, not a public key. The same anti-junk predicates the other human-typed handles use apply, so a held key and a one-character masher are refused.
  ```go
  type RoleKey string
  func (v RoleKey) Value() string { return string(v) }
  func (v RoleKey) IsValid(fieldName string, ctx *domain.NotificationContext) bool
  ```
  The underlying type is `string` and is not negotiable: the mappers convert with `vos.RoleKey(x)` and read back with `.Value()`. `IsValid` is the framework's entry point — it is found by TYPE, with no registration, and reports every problem it finds through the context rather than returning one, so a caller sees all of them at once.

### `internal/domain/role_rules_manual.go`

The spec declared these invariants as ones it could not express. The file was just created, with a stub for each; the code is yours to write, and regeneration will never touch it.

**`tenant-must-be-active`**

> The owner tenant must exist and not be archived. Ask the TenantIsUnavailable fact with the role's TenantID; when it answers true, refuse. On insert only — the tenant is immutable afterwards, so a later archive of the tenant must not freeze edits to roles that already exist under it.

- fires under `IfInsert` · raise `RoleTenantDoesNotExistNotification{}` · attach it to `TenantID`

**`no-wildcard-grant`**

> No granted permission may carry a wildcard in either part. For each entry of Permissions, ask the PermissionIsWildcard fact with the entry's PermissionID and refuse when it answers true. This rule MUST be evaluated before caller-must-hold-granted-permission, and is what guarantees no wildcard string ever reaches Identity.HasPermission, which panics on one. Consequence, accepted: the platform's own *:* role is seeded by migration beside the reserved platform tenant, not created through this API.

- fires under `IfInsertOrUpdate` · raise `CannotGrantWildcardPermissionNotification{}` · attach it to `Permissions`

**`granted-permission-must-be-in-catalog`**

> Every granted permission must exist in the catalog and be active. For each entry of Permissions, ask the PermissionIsNotInCatalog fact with the entry's PermissionID and refuse when it answers true. The database foreign key already guarantees EXISTENCE; this rule earns its keep for the ACTIVE half, and for turning a violation into a readable 422 instead of a raw constraint error.

- fires under `IfInsertOrUpdate` · raise `PermissionNotInCatalogNotification{}` · attach it to `Permissions`

**`caller-must-hold-granted-permission`**

> No privilege escalation: a caller may only grant a permission they themselves hold. For each entry of Permissions, ask the CallerDoesNotHoldPermission fact with the entry's PermissionID and refuse when it answers true. Evaluate this AFTER no-wildcard-grant, so every key reaching it is concrete. A *:* super-admin needs no special case — Identity.HasPermission answers true for any concrete permission when the claim set carries *:*. TWO absent states, and collapsing them breaks one profile or the other: no Identity at all (auth.mode: disabled, dev only) means this rule stands down; an Identity present whose claim is empty or insufficient is REFUSED, fail closed.

- fires under `IfInsertOrUpdate` · raise `CannotGrantUnheldPermissionNotification{}` · attach it to `Permissions`

Its tests are yours too — the generator does not know what these rules mean.

### `internal/infra/role_service_manual.go`

The spec marked these questions as ones the generator cannot answer, so it declared them on the port and left the bodies to you. **They panic until you write them** — the project still builds and boots, and the failure arrives the moment the rule asks, as a 500 with the write rolled back. The outcome being avoided is the other one: a query against the wrong source would compile, return, and mean nothing.

**`TenantIsUnavailable(tenantID domain.ID) bool`**

> Whether the owner tenant is unknown or archived. Not answerable from this entity's own table — it is a question about the tenants table, so the implementation holds the tenant repository beside its own.

**`PermissionIsNotInCatalog(permissionID domain.ID) bool`**

> Whether this granted permission is absent from the catalog or archived. Asked once per entry. Not answerable declaratively — the column lives on the permissions table, not on this entity's.

**`PermissionIsWildcard(permissionID domain.ID) bool`**

> Whether the catalog row this grant points at carries a wildcard in either part. Asked once per entry, and answered by resolving the id to its vos.PermissionKey. It is what keeps every wildcard string away from Identity.HasPermission, which panics on one — so it must answer TRUE for an unknown id too, rather than let an unresolvable grant fall through to the caller-holds question.

**`CallerDoesNotHoldPermission(permissionID domain.ID) bool`**

> Whether the authenticated caller lacks the permission this grant points at. Asked once per entry, and only for concrete keys — no-wildcard-grant has already refused the wildcards. Implementation: resolve the id to its key; return false when ctx.Identity() is nil (auth disabled, dev only) so the rule stands down; otherwise answer NOT Identity.HasPermission(key). Guard the wildcard here as well and answer true rather than calling through — a panic on a security rule is a 500.

The method returns a plain value and no error, so decide what an unavailable source means. Failing loudly is the safe default — returning a plausible answer skips the rule this exists to enforce.

### Per-entry command tests are generated now

The verbs that address ONE entry — add, remove — have generated tests in `internal/application/commands/role_commands_test.go`: the entry is applied and projected back, a change keeps its id, an unknown id projects nothing.

**If you wrote your own tests for those mappers before this run**, the package will not compile until you delete them — Go reports it as `redeclared in this block`, which reads like a generator bug and is not one. The generated cases cover the same ground; anything yours asserts beyond them is worth keeping under a different name.

## What to check

These are the decisions the spec made that are expensive to change later. Read them against what you actually meant.

| Decision | Value | Why it matters |
|---|---|---|
| Storage | flat table `roles` | A field group that should be shared with another role later would need a real migration to extract. |
| Operations | `insert`, `patch`, `archive`, `byParams`, `byId` | Each one is a route with a permission; an unwanted one is a surface you did not mean to expose. |
| Removal | archive (reversible) | `DELETE` is a permanent purge and is not mounted. |
| Unique | `Key` — per TenantID, scope `active-only` (service-precheck+constraint) | an archived row frees it, so the value can be taken again; a duplicate is refused at the database and reported as `RoleKeyAlreadyExistsNotification`. |
| Data access | tenant | Callers are restricted to their tenant's rows. |
| Crossing the scope | `*:*` | Only a super-admin crosses the scope, and nothing new became grantable — what crosses is the claim they already carry. The wildcard cannot be handed to the framework's HasPermission (it panics on one), so the generated guard calls `Identity.IsSuperAdmin()` instead — the framework's own question for the `*:*` grant, nil-safe and honouring the configured permissions claim. A resource wildcard like `role:*` does NOT answer it. |
| Read backing | relational | Reads come straight from the tables, so a write is visible immediately. |

## What was generated

| What | File |
|---|---|
| the roles feature (repository + view + mount) | `bootstrap/roles_feature.go` |
| the RolesFeature registration in the composition root | `bootstrap/wire.go` |
| the archive command and result | `internal/application/commands/archive_role_command.go` |
| the insert command and result | `internal/application/commands/insert_role_command.go` |
| the patch command and result | `internal/application/commands/patch_role_command.go` |
| the shapes for 1 child collection(s) | `internal/application/commands/role_child_results.go` |
| tests for the command mappers | `internal/application/commands/role_commands_test.go` |
| the per-entry commands for role_permissions | `internal/application/commands/role_permission_commands.go` |
| tests for the 1 collection input mapper(s) | `internal/application/dtos/role_dtos_test.go` |
| the RolePermission input DTO | `internal/application/dtos/role_permission_input.go` |
| the by-id query and its result | `internal/application/queries/find_role_by_id_query.go` |
| the listing query and its result | `internal/application/queries/find_roles_by_params_query.go` |
| the read criteria tests | `internal/application/queries/role_queries_test.go` |
| the read shapes for 1 child collection(s) | `internal/application/queries/role_row_results.go` |
| 16 DEU translation key(s) | `internal/application/translations/deu.go` |
| 16 ENG translation key(s) | `internal/application/translations/eng.go` |
| 16 ESP translation key(s) | `internal/application/translations/esp.go` |
| 16 FRA translation key(s) | `internal/application/translations/fra.go` |
| 16 ITA translation key(s) | `internal/application/translations/ita.go` |
| 16 NLD translation key(s) | `internal/application/translations/nld.go` |
| 16 PTBR translation key(s) | `internal/application/translations/ptbr.go` |
| the translation coverage test — every notification must be translatable in every catalog | `internal/application/translations/role_translations_test.go` |
| tests for the collection types | `internal/domain/aggregatevos/role_children_test.go` |
| the RolePermission child value object | `internal/domain/aggregatevos/role_permission.go` |
| 9 notification declaration(s) | `internal/domain/notifications.go` |
| the Role aggregate root, its modes and its rules | `internal/domain/role.go` |
| the hand-written rules for Role (4 to implement) | `internal/domain/role_rules_manual.go` |
| the Role service port (5 fact(s)) | `internal/domain/role_service.go` |
| tests for Role's rules | `internal/domain/role_test.go` |
| the vos package documentation | `internal/domain/vos/doc.go` |
| 1 notification declaration(s) | `internal/domain/vos/notifications.go` |
| the Role repository and its constraint bindings | `internal/infra/role_repository.go` |
| the Role service implementation | `internal/infra/role_service.go` |
| the hand-written facts for Role (4 to implement) | `internal/infra/role_service_manual.go` |
| the role_permissions child schema | `internal/infra/schemas/role_permission_schema.go` |
| the roles schema (4 columns) | `internal/infra/schemas/role_schema.go` |
| the schema builder tests — they run the builders, so a boot panic is a test failure | `internal/infra/schemas/role_schemas_test.go` |
| the roles view (relational-backed) | `internal/infra/views/role_view.go` |
| the view definition test — it builds the definition, so a boot panic is a test failure | `internal/infra/views/role_view_test.go` |
| the by-id request and response | `internal/web/requests/find_role_by_id.go` |
| the listing request and response | `internal/web/requests/find_roles_by_params.go` |
| the insert request and response | `internal/web/requests/insert_role.go` |
| the patch request and response | `internal/web/requests/patch_role.go` |
| the wire types for 1 child collection(s) | `internal/web/requests/role_children.go` |
| the per-entry wire types for role_permissions | `internal/web/requests/role_permission_requests.go` |
| the request mapper tests | `internal/web/requests/role_requests_test.go` |
| the 5 role endpoints | `internal/web/role_routes.go` |
| the rollback of roles on postgres | `migrations/postgres/0003_role_manual.down.sql` |
| the roles table on postgres | `migrations/postgres/0003_role_manual.up.sql` |

## What was NOT generated

Owned by other tools:

- the gRPC surface and its proto contract — `/omnicore:implement`
- integration events (publish/subscribe) — `/omnicore:implement`
- read models spanning more than this entity — `/omnicore:scaffold-view`
- changing this entity once it exists — `/omnicore:evolve-entity`, which edits this spec and regenerates. The CODE comes back from the spec; the DATABASE never does — the migration a change needs is written by hand, and that skill's impact map is what carries it, along with the orphans a shrinking spec leaves and everything outside this generator's ownership

Read controls this listing does NOT serve: `?search=`. That is a contract, not an omission — sending one is answered with a typed 400 rather than being ignored.

## Framework compatibility and next steps

Verdict: **exact** (project pins v0.56.0)

framework v0.56.0 meets the required v0.56.0

Verify what was generated:

```
go build -tags 'postgres' ./...
go vet -tags 'postgres' ./...
go test -tags 'postgres' ./... -count=1
```

A service that builds with a transport tag (kafka, nats) needs it IN ADDITION to the engine tag on every command above — an engine tag alone may not select a buildable configuration there.

Then exercise the endpoints end to end — a green build proves the code compiles, not that the entity works.
