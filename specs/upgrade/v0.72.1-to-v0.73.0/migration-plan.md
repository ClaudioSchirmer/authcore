# Migration plan — omnicore v0.72.1 → v0.73.0

Status: APPROVED

Decision on the open item (§1): **regenerate all seven** — option (a). The five entities
still emitted at framework v0.69.0 stop drifting, and the OpenAPI document gains the
`description:"..."` parameter tags on this bump rather than a wider diff on the next one.

The `go.mod` / `go.sum` bump was already done; the build was RED and this plan is what
made it green again.

Service: `authcore` · Build tags: `postgres` (engine from `relational.dialect`; neither
profile declares a `transport:` block)

Rollback point: `specs/upgrade/v0.72.1-to-v0.73.0/rollback/` (verbatim `go.mod` + `go.sum`
at `v0.72.1`, taken 2026-09-06, byte-identical by `shasum`).

---

## Context — one release, two breaking changes, both landing here

v0.73.0 carries exactly two breaking items and this service is exposed to both.

1. **Notifications are emitted by FIELD REFERENCE; the string-named `AddNotification` is
   gone.** Source-breaking. 133 call sites across 27 files.
2. **The wire `field` token is camelCase everywhere the FRAMEWORK names a field.**
   Behavioral. It does not break the build — and, on this service, it does not break the
   contract either: it *repairs* it. See §7.

**Operational classes checked and clear**, by diffing the two module trees:

- No yaml key moved, renamed or arrived mandatory — `application/configuration` differs
  only in `language_test.go` between the pins. Both profiles stay valid as written.
- No embedded migration added — the framework's `.sql` set is identical, so no
  `autoRun: check` boot abort is coming.
- No DDL on this service's own tables.
- No gRPC proto fallout — this service mounts no gRPC surface and ships no `.proto`.
- No adopted generated file — `specs/omnicore-gen/lock.json` records zero adoptions, so
  every emitter improvement in the range reaches this project through regeneration.

### The contract, at both pins

Read at `docs/content/sections/rules-dsl.html` and `status-mapping.html` in each module
directory.

| | v0.72.1 | v0.73.0 |
|---|---|---|
| Rule seat | `r.AddNotification(name string, n, value ...any)` | `r.AddNotification(field any, n, exposeValue bool)` |
| String seat on `Rules` | — (the same one) | `r.AddNotificationNamed(name string, n, value ...any)` |
| String seat on `NotificationContext` | `ctx.AddNotification(name, n, value ...)` | `ctx.AddNotificationNamed(name, n, value ...)` |
| String seat on `BaseEntity` | `e.AddNotification(name, n, value ...)` | `e.AddNotificationNamed(name, n, value ...)` |
| AVO `BuildRules` receiver | value — `func (c UserRole) BuildRules(...)` | **pointer** — `func (c *UserRole) BuildRules(...)`, and the method left the `AggregateValueObject` interface |
| `NotificationMessage.FieldName` | present (legacy literal slot) | **removed** — `Override` (literal) and `Path` (rendered) are the only slots |
| Manual `Rules` constructor | `NewRules(mode, ctx, entityType)` | `NewRulesFor(mode, ctx, base)` binds a base pointer so field references resolve; `NewRules` survives for name-only seats |

`labelKey:"..."` is **unchanged** — it was already the tag name at v0.72.1
(`domain/field_label.go:91`), and all 56 declarations in this service keep working. The
`notifyAs:"..."` tag is NEW and purely additive; no edit below uses it.

---

## §1 — Regenerate the seven entities

**Verified, not assumed:** the seven specs were regenerated in a throwaway copy of this
project at v0.73.0 and the result was compiled. The generator emits the new API correctly
and makes the field-ref/named split itself.

`omnicore-gen` **requires framework v0.73.0 or later** — at the old pin `check` refused
outright (*"the project pins framework v0.72.1, older than the v0.73.0 this generator
requires… generation is blocked"*). The bump is what unblocks it.

Proposed: run, from the project root, for each of the seven specs —

```
omnicore-gen generate -spec specs/omnicore-gen/<entity>.omnicore.yaml
```

`tenant`, `permission`, `claim`, `client`, `group`, `role`, `user`. Nothing is created and
nothing is deleted: 29 files updated, 5 kept as-is per entity ("yours, by design").

This alone fixes **61 of the 133 call sites**, all 8 aggregate-VO receivers, and the seven
generated `*_test.go` helpers. Sample of what it writes, from the probe on `role.go`:

```go
r.AddNotification(&e.Key, RoleKeyIsImmutableNotification{}, true)          // real field → reference
r.AddNotification(&e.TenantID, notifications.TenantMismatchNotification{}, false)
r.AddNotificationNamed("Permissions", RoleAlreadyGrantsPermissionNotification{}, ...)  // collection → named
```

```go
func (c *RolePermission) BuildRules(actionName string, service domain.Service, r *domain.Rules) {   // pointer
```

### Decided: regenerate all seven — option (a)

`omnicore-gen doctor` reports five of the seven were last generated at framework
**v0.69.0** (`claim`, `client`, `group`, `tenant`, `user`); only `permission` and `role` are
at v0.72.1. Regenerating the five therefore jumps them four framework versions, so the
diff is wider than the notification API. I measured that surplus — it is **one** thing,
and it is additive:

`internal/web/requests/find_{claims,clients,groups,tenants,users}_by_params.go` gain
`description:"..."` struct tags on every query parameter, sourced from the field comments
in the entity. 16–30 lines each. They feed the OpenAPI document; no filter, no sort and no
response shape changes. `find_permissions_by_params.go` and `find_roles_by_params.go` do
not move, which is what confirms this is v0.69.0→v0.73.0 drift and not a v0.73.0 change.

- **(a) Regenerate all seven** — the five stop drifting, the OpenAPI document gains the
  parameter descriptions. The published OpenAPI changes on this bump.
- **(b) Regenerate only what the build needs** — the same seven still have to be
  regenerated (all seven emit notifications), so this option really means *regenerate all
  seven and revert the five `find_*_by_params.go` files by hand*, which pins them at the
  v0.69.0 shape and widens the drift at the next upgrade.

(a) was chosen. It costs one OpenAPI diff now instead of a bigger one later, and nothing
about it is behavioral.

---

## §2 — `internal/domain/vos/` — the shared value objects

**Error (verbatim, 28 of them):**

```
internal/domain/vos/cidr_block.go:63:7: ctx.AddNotification undefined (type *domain.NotificationContext has no field or method AddNotification)
```

**How it worked at v0.72.1** — `ctx.AddNotification(name, n, value...)` built
`Path: []PathSegment{{Name: name}}` and resolved `LabelKey` from the entity type.

**How it works at v0.73.0** — the same method is named `AddNotificationNamed`. Its body is
the old one plus a line: it now also resolves the field's `notifyAs` tag into
`msg.Path[0].Wire`. **The wire output is identical** for every field without that tag,
which is all 56 of them here.

These helpers cannot take a field reference and are not supposed to: `rules-dsl` at
v0.73.0 says *"a raw or enum value object is generic and shared, so its name always comes
from the field that holds it"*. The named seat is the documented landing, not a fallback.

**Proposed edit** — a pure rename, `ctx.AddNotification(` → `ctx.AddNotificationNamed(`,
in the 10 hand-written files (28 sites):

| file | sites |
|---|---|
| `cidr_block.go` | 4 |
| `claim_name.go` | 3 |
| `description.go` | 2 |
| `display_name.go` | 2 |
| `group_key.go` | 2 |
| `password.go` | 2 |
| `permission_key.go` | 6 |
| `person_name.go` | 2 |
| `role_key.go` | 2 |
| `tenant_workspace.go` | 3 |

`vos/email.go` and `vos/claim_value.go` are generated and are covered by §1.

---

## §3 — `internal/domain/*_rules_manual.go` — the hand-written rules

44 sites in 8 files — 20 become field references, 24 become `AddNotificationNamed`.
Every one passes a string literal, so the split is decidable by
reading the entity struct: a name that IS an addressable field becomes a reference; a name
that is not becomes `AddNotificationNamed`. The `exposeValue` flag follows the old call
mechanically — a call that passed a third argument echoed a value and becomes `true`; one
that passed none becomes `false`.

**Verified against the structs**, every value echoed at a field-ref site is that field's
own value, so `exposeValue: true` reproduces it exactly.

### → field reference (20 sites)

| file:line | name | becomes | expose |
|---|---|---|---|
| `claim_rules_manual.go:56` | `TenantID` | `&e.TenantID` | false |
| `claim_rules_manual.go:80` | `DefaultValue` | `&e.DefaultValue` | true |
| `claim_rules_manual.go:218,222,360,370,374` | `AppliesTo` | `&e.AppliesTo` | true |
| `client_rules_manual.go:200` | `GracePeriodSeconds` | `&e.GracePeriodSeconds` | true |
| `client_rules_manual.go:211` | `Status` | `&e.Status` | true |
| `client_rules_manual.go:262` | `TenantID` | `&e.TenantID` | true |
| `group_rules_manual.go:62` | `TenantID` | `&e.TenantID` | true |
| `permission_rules_manual.go:59` | `Description` | `&e.Description` | true |
| `role_rules_manual.go:55` | `TenantID` | `&e.TenantID` | true |
| `tenant_rules_manual.go:46` | `Description` | `&e.Description` | true |
| `user_rules_manual.go:187` | `Password` | `&e.Password` | false |
| `user_rules_manual.go:208` | `TenantID` | `&e.TenantID` | true |
| `user_credential_manual.go:126` | `CurrentPassword` | `&e.CurrentPassword` | false |
| `user_credential_manual.go:140,175` | `Password` | `&e.Password` | false |
| `user_credential_manual.go:225` | `PasswordConfirmation` | `&e.PasswordConfirmation` | false |

The three credential fields keep `exposeValue: false` — they carry secrets and the old
calls echoed nothing. `true` there would put a plaintext password in a 422 body.

### → `AddNotificationNamed` (24 sites)

`ID` is not a field: it lives unexported inside the embedded `Managed`, reachable only as
`GetID()`, so `&e.ID` does not compile. The collections have no slice field either — the
generated entities state it outright (*"No slice field for the children, deliberately: the
framework keeps them in its own collection"*), which is also why §1's generator chose
`AddNotificationNamed` for them.

| name | sites |
|---|---|
| `ID` | `client_rules_manual.go:313`, `user_credential_manual.go:108,161` |
| `Roles` | `client_rules_manual.go:400,414,424`, `group_rules_manual.go:186,213,231`, `user_rules_manual.go:369,375,381` |
| `Claims` | `client_rules_manual.go:525,534,544`, `user_rules_manual.go:474,487,500` |
| `Groups` | `user_rules_manual.go:297,314,324` |
| `Permissions` | `role_rules_manual.go:164,181,195` |

These keep their `value ...any` third argument verbatim — the echoed id belongs to the
rejected child, not to a field of the root, so only the named seat can carry it.

**No wire change from this section.** Both seats have always written the name into
`Path[0].Name`, and `Path` was already rendered lowerCamel at v0.72.1.

---

## §4 — `NotificationMessage.FieldName` is removed

**File:** `internal/application/commands/handlers/utils/authentication.go:95`

```go
return exception.NewApplicationErrorWith("Authentication", domain.NotificationMessage{
    FieldName:    "credentials",
    Notification: n,
})
```

**v0.72.1** — `FieldName` was the legacy literal slot, last in the
`Override > rendered Path > FieldName` precedence, emitted verbatim.

**v0.73.0** — the slot is gone. `status-mapping` at the new pin: *"a field name the SERVICE
supplies (`SingleNotificationError`, `ConstraintBinding.Field`, a manual
`NotificationMessage`) lands in `Override` and travels verbatim — declare it in wire
casing"*. `"credentials"` is already wire casing.

**Proposed edit:** `FieldName:` → `Override:`. One line. The wire keeps saying
`"field": "credentials"`, which is what the `Refusal` doc comment above it promises and
what the QA security lane asserts.

---

## §5 — Test files that read `FieldName`

Three hand-written test files read the removed slot:

- `internal/application/commands/handlers/issue_token_command_handler_test.go:311,313,882,883`
- `internal/application/commands/handlers/issue_client_token_command_handler_test.go:164,166`
- `internal/application/commands/handlers/change_password_command_handler_test.go:292` (a
  comment only — reworded, no code change)

**Proposed edit:** read `msg.Override` where the assertion is about the literal
`"credentials"` §4 sets. The seven `internal/domain/*_test.go` readers are generated and
§1 rewrites them — the probe shows the generator collapsing the old three-branch resolver
to `name := msg.Override`.

---

## §6 — `internal/domain/vos/user_manual_vos_test.go`

No edit. The single `AddNotification` hit is inside a comment (line 184). Listed here so
the file is not mistaken for an omission.

---

## §7 — The wire change, and why it closes two open failures

Breaking item 2 is behavioral and I found no edit this service needs for it. What it does
is **fix the two failures the QA suite is currently red on.**

`qa/qa-report.md` (run 20260903-233433, at v0.72.1) — ❌ RED, 1 of 5 suites, exactly two
cases:

```
D12a tenantID: empty
  expected: HTTP 422 · InvalidIDUUIDNotification · field=tenantID
  received: HTTP 422 · InvalidIDUUIDNotification · field=TenantID
```

That notification is framework-raised, from `domain/id.go`. The diff between the pins is
one line:

```diff
-			FieldName:    fieldName,
+			Path:         []PathSegment{{Name: fieldName}},
```

`FieldName` was verbatim; `Path` is rendered acronym-aware lowerCamel. `TenantID` →
`tenantID` — precisely what the suite asserts. **No QA edit is proposed.** Both cases
should pass on the new pin, and running the suite is how that is confirmed, not this file.

Every other asserted `field` token in the five suites was checked and is already wire-cased
(`key`, `tenantID`, `resource`, `orderBy[bogus]`, `description.eq`, …). None is Go-cased or
a physical column, so every emitter this release re-cased moves **toward** what the suites
assert, never away.

---

## §8 — Found at VERIFICATION, not proposed upfront

Two things surfaced only when `vet` and the unit suite ran. Both are direct consequences
of the approved change; both were applied and are recorded here so the plan matches what
the repository actually contains.

### §8.1 — `aggregatevos/role_permission_manual_test.go:69`

```
vet: cannot call pointer method BuildRules on RolePermission
```

`RolePermission{}.BuildRules(...)` on a composite literal is not addressable once the
method takes a pointer receiver (§1). Applied:

```go
entry := &RolePermission{}
r := domain.NewRulesFor(domain.ModeInsert, ctx, entry)
entry.BuildRules("GetInsertable", nil, r)
```

`NewRulesFor` replaces `NewRules(…, nil)` here so the `Rules` is bound to a base, which is
how the framework itself invokes it — the day a rule is added to this entry, a field
reference in it resolves instead of panicking.

`group_children_manual_test.go:151` needed no change: its `entry` is a variable, so Go
takes its address implicitly.

### §8.2 — The child-duplicate rejection changed which name it blames

Two hand-written tests failed:

```
group_children_manual_test.go:93: the refusal blamed [Roles], want GroupRole
role_children_manual_test.go:201: the refusal blamed [Permissions], want RolePermission
```

**v0.72.1** — `domain/aggregate_root.go` emitted the duplicate/not-found/invalid-child
rejections with `FieldName: key`, where `key` was the Go TYPE name of the entry.

**v0.73.0** — the same four seats emit `Path: childFieldPath(item)`, built from the entry's
declared `CollectionName()`. `GroupRole` → `Roles`, `RolePermission` → `Permissions`. On
`rejectChild` the Go type name survives as `FieldValue`, the diagnostic echo.

Both tests were asserting the OLD contract, and their own NOTE comments described it as an
anomaly — *"the generated childDuplicate rule blames 'GroupRole' — the ENTRY TYPE — where
every other refusal about this collection blames 'Roles', the collection"*. v0.73.0 ended
exactly that split. Applied: each assertion now expects the collection, and each NOTE was
rewritten to describe the uniform contract instead of the anomaly.

**This is a wire change on the aggregate-child routes** — a duplicate grant now answers
`"field": "roles"` where it answered `"field": "GroupRole"`. No QA suite asserts either
token today; the collection suites in `qa/role.sh` cover GRANT/REVOKE but not the
duplicate-entry rejection.

---

## Applied — result

`go build -tags postgres ./...` and `go vet -tags postgres ./...` both exit 0.
`go test -tags postgres ./...` is green across all 14 packages.

---

## Needs your attention — not auto-fixable, no edit proposed

- **Framework emitters that used to name a physical column now name the Go field.** The
  shared-base duplicate guard and the not-found/concurrency probes moved from
  `schema.IDColumn()` to `schema.WireFieldOf(schema.IDColumn())`
  (`infra/db/command/write/shared_base_write.go:224,352,710`, `flat_write.go:139,335`,
  `aggregate_write.go:118,414`, `direct_write.go:706`, `read/direct_repository.go:144`,
  `read/aggregate_loader.go:633`), and the natural-key-immutable guard from the Go name to
  the wire name (`shared_base_write.go:425`). Any consumer or script matching the old token
  must update. This service's own suites do not.
- **`ReadCriteria.Restrict`'s 403 and the read-side 400s now carry the wire path.**
  `application/queries/view_reader.go:90` and `infra/db/core/exception.go:98,117` render
  through `domain.WireFieldPath`, so a refusal about a joined leaf answers
  `"addresses.zipCode"` where it used to answer `"Addresses.ZipCode"`. Nothing in this
  service asserts the Go-cased form, but the ACCESS_MATRIX prose and any external client
  written against it should be re-read.
- **`notifyAs:"..."` is available and unused.** Any field whose wire token should differ
  from its camelized Go name can now declare it once on the struct instead of through
  `AddFieldNameAlias` in a constructor. Nothing here needs it today; it is the cheaper seat
  the next time one does.
- **A green build is not a green boot.** vet and build under `-tags postgres` prove the Go
  surface compiles. The field-reference resolver **panics at runtime** on a reference it
  cannot honor — a non-pointer, a pointer field passed without `&`, a reference into a copy
  — and a declared child type whose `*T` lacks `BuildRules` panics when
  `AggregateChildren()` is first consulted. Only running the service and the suite
  exercises any of that.
