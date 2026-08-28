# Access Matrix

Who reaches what, per endpoint. The code as it stands.

Status: **in progress**. Mapped: Permission, Tenant, Role, Group, User, Client.
Pending: Authentication.

## Columns

| Column | Reads |
|---|---|
| **Permission** | What `RequirePermission` demands at the mount. Satisfied exactly, by resource wildcard (`x:*`), or by `*:*` |
| **Admission** | `JWT + claim` = bearer token AND a `tenant_id` claim, both required to reach the handler. Service-wide gate: 403 without it, no permission bypasses it |
| **Tenant scope** | What ties the row to the caller's tenant. `filter TenantID` = the read query forces `Filter["TenantID"] = id.TenantID()`. `guard foreign-tenant` = the domain refuses a write whose row is not the caller's (`TenantMismatchNotification`). `none` = nothing does |
| **Self** | What the caller's IDENTITY says about the target ROW, and for WHICH kind of caller. `sub → self` / `sub → not self` = the domain compares the path id against the caller's subject, whoever they are. `kind:client → …` applies **only to a client-subject token**: a user token of the same tenant never meets it and passes on the tenant scope alone. `—` = nothing compares them |
| **`*:*` crosses** | Whether `IsSuperAdmin()` lifts the tenant scope. It never lifts a **Self** rule — that comparison does not ask |
| **Rows reached** | What the caller ends up touching |

---

## Permission

`permissions` has no `tenant_id` column.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /permissions` | `createPermission` | `permission:insert` | JWT + claim | none | — | n/a | creates |
| `PATCH /permissions/:id` | `patchPermission` | `permission:update` | JWT + claim | none | — | n/a | any row |
| `PATCH /permissions/:id/archive` | `archivePermission` | `permission:archive` | JWT + claim | none | — | n/a | any row |
| `GET /permissions` | `permissions` | `permission:read` | JWT + claim | none | — | n/a | all rows |
| `GET /permissions/:id` | `permission` | `permission:read` | JWT + claim | none | — | n/a | any row |

No unarchive mounted. Update carries `Description` only — `resource` and `action` are reachable by no request after creation.

---

## Tenant

The isolation partition itself. The row id is the `tenant_id` claim.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /tenants` | `createTenant` | `tenant:insert` | JWT + claim | none | — | n/a | creates |
| `PATCH /tenants/:id` | `patchTenant` | `tenant:update` | JWT + claim | **none** | — | n/a | **any tenant** |
| `PATCH /tenants/:id/archive` | `archiveTenant` | `tenant:archive` | JWT + claim | **none** | — | n/a | **any tenant** |
| `PATCH /tenants/:id/unarchive` | `unarchiveTenant` | `tenant:archive` | JWT + claim | **none** | — | n/a | **any tenant** |
| `GET /tenants` | `tenants` | `tenant:read` | JWT + claim | **none** | — | n/a | **all tenants** |
| `GET /tenants/:id` | `tenant` | `tenant:read` | JWT + claim | **none** | — | n/a | **any tenant** |

Both `ToCriteria` return the criteria unchanged; the four write commands bind no identity. Nothing compares the target row against the caller's `tenant_id` — containment is the distribution of `tenant:read` / `tenant:update` / `tenant:archive` and nothing else.

Archive and unarchive share `tenant:archive` — one permission, both directions. Archive also forces `Status` to `suspended`.

Update carries `Name`, `Description`, `Status` — `Workspace` is reachable by no request after creation.

---

## Role

Tenant-scoped: `roles.tenant_id`. Owns the `permissions` collection — the grants.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /roles` | `createRole` | `role:insert` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /roles/:id` | `patchRole` | `role:update` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /roles/:id/archive` | `archiveRole` | `role:archive` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `POST /roles/:id/permissions` | `addRolePermission` | **`role:grant`** | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /roles/:id/permissions/:rolePermissionId/archive` | `removeRolePermission` | **`role:grant`** | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `GET /roles` | `roles` | `role:read` | JWT + claim | filter TenantID | — | yes | own tenant |
| `GET /roles/:id` | `role` | `role:read` | JWT + claim | filter TenantID | — | yes | own tenant |

The two collection endpoints answer on BOTH surfaces, same command and same permission. The one difference is the shape, not the reach: on GraphQL the entry id rides the input (`rolePermissionId`) rather than a path segment, and the revoke resolves to `success: true` where REST answers `204`.

Granting and revoking ride `role:grant`, a verb of their own — not the root's `role:update`. A principal holding `role:update` alone may relabel a role and gets **403** on both collection endpoints. Same split as `Group` and `User`: "may rename it" and "may change what it confers" are separately grantable across the whole service.

`tenantID` travels in the insert and patch bodies. On insert the guard refuses any value that is not the caller's; on update `RoleTenantIsImmutableNotification` refuses any change to it. A role never moves between tenants.

Writes load the row through the repository, which the read filter never touches — the guard is what stands between a caller and another tenant's role.

### What a role may be granted

Judged on the entries a write ADDS — on insert that is all of them; a revoke adds none and is judged by none.

| Rule | Effect |
|---|---|
| in-catalog | The permission id must exist and be active. A retired permission returns as a new id, so re-granting the old one is refused |
| no-wildcard | A permission with `*` in either part cannot be granted through this API, on any role |
| no-escalation | A caller may only grant a permission they themselves hold. `*:*` satisfies every concrete permission, so a super-admin grants anything |

The escalation gate stands down only when the request carried no identity at all — a dev bench. An identity present with an insufficient claim still refuses.

---

## Group

Tenant-scoped: `groups.tenant_id`. Owns the `roles` collection — the bundle of bundles a member inherits by belonging.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /groups` | `createGroup` | `group:insert` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /groups/:id` | `patchGroup` | `group:update` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /groups/:id/archive` | `archiveGroup` | `group:archive` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `POST /groups/:id/roles` | `addGroupRole` | **`group:grant`** | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /groups/:id/roles/:groupRoleId/archive` | `removeGroupRole` | **`group:grant`** | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `GET /groups` | `groups` | `group:read` | JWT + claim | filter TenantID | — | yes | own tenant |
| `GET /groups/:id` | `group` | `group:read` | JWT + claim | filter TenantID | — | yes | own tenant |

The two collection endpoints answer on BOTH surfaces, same command and same permission — same shape difference as `Role`'s: on GraphQL the entry id rides the input (`groupRoleId`) rather than a path segment, and the detach resolves to `success: true` where REST answers `204`.

`group:grant` is a fifth verb, the same shape `Role` and `User` carry: a collection endpoint never rides its root's update. A principal holding `group:update` alone may rename and re-describe a group and gets **403** on both collection endpoints; a principal holding only `group:grant` may attach and detach on a group it cannot rename.

No unarchive mounted, on the root or per entry. A detached entry does not come back: attaching the same role again mints a NEW entry with a new id.

`tenantID` travels in the insert and the patch bodies; `key` travels in both as well. On insert the guard refuses a `tenantID` that is not the caller's; on update `GroupTenantIsImmutableNotification` and `GroupKeyIsImmutableNotification` refuse any change to either. So the PATCH contract advertises two fields the domain then refuses — the same asymmetry that `patchExcludes: [Key]` closed on Permission. `Name` and `Description` are what an update actually changes.

Writes load the row through the repository, which the read filter never touches — the guard is what stands between a caller and another tenant's group.

### What a group may be granted

Judged on the entries a write ATTACHES — on insert that is all of them; a detach adds none and is judged by none.

| Rule | Effect |
|---|---|
| tenant-must-be-available | Insert only. The owning tenant must exist, must not be archived and must not be `suspended`. A TRIAL tenant passes: unavailable is not the same question as not active |
| attached-roles-are-available-in-this-tenant | The role must be in the table, still active, AND owned by the tenant. One notification for all three — a distinct "belongs to another tenant" reply would confirm that a given UUID is a live role in some other tenant. The tenant compared is the GROUP's, not the caller's, which is what keeps a crossing `*:*` operator attaching the customer's own roles |
| no-wildcard-role-attach | A role conferring any permission with `*` in either part cannot be attached through this API, on any group. Runs BEFORE the escalation rule — `HasPermission` panics on a wildcard argument — and AFTER the availability rule, because an unresolvable role answers "wildcard" too. The consequence: the platform's own superadmin group is not creatable through this API |
| no-privilege-escalation | TRANSITIVE: a caller may confer a role only if they hold EVERY permission that role grants — a set, not one key, which is what separates this from Role's rule one level down. `*:*` satisfies every concrete permission, so a super-admin confers anything |

The escalation gate stands down only when the request carried no identity at all — a dev bench. An identity present with an insufficient claim still refuses.

At most 50 roles in one group, no duplicates. The business identity of an entry is the role reference alone.

Membership is not here: `user_groups` is a collection of **User**, gated on `user:grant`.

---

## User

Tenant-scoped: `users.tenant_id`. Two collections (`groups`, `roles`) and the two hand-written credential routes — the only endpoints in the service where the caller's own identity decides the row.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached | Must change password |
|---|---|---|---|---|---|---|---|---|
| `POST /users` | `createUser` | `user:insert` | JWT + claim | guard foreign-tenant | — | yes | own tenant | **next sign-in** |
| `PATCH /users/:id` | `patchUser` | `user:update` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `PATCH /users/:id/archive` | `archiveUser` | `user:archive` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `POST /users/:id/groups` | `addUserGroup` | `user:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `PATCH /users/:id/groups/:userGroupId/archive` | `removeUserGroup` | `user:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `POST /users/:id/roles` | `addUserRole` | `user:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `PATCH /users/:id/roles/:userRoleId/archive` | `removeUserRole` | `user:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant | — |
| `GET /users` | `users` | `user:read` | JWT + claim | filter TenantID | — | yes | own tenant | — |
| `GET /users/:id` | `user` | `user:read` | JWT + claim | filter TenantID | — | yes | own tenant | — |
| `PATCH /users/:id/password` | `changeUserPassword` | `user:change-password` | JWT + claim | guard foreign-tenant | `sub → self` | yes | **own row** | — |
| `PATCH /users/:id/password-reset` | `resetUserPassword` | `user:reset-password` | JWT + claim | guard foreign-tenant | `sub → not self` | yes | any OTHER user, own tenant | **next sign-in** |

- **Must change password**: the endpoint leaves a password the user did not choose, so the next sign-in has to rotate it. The token that sign-in issues carries `user:change-password` and nothing else, whatever the account holds — see *By the TOKEN itself*.
- **Insert**: `tenantID` is optional in the body. Absent means the claim's tenant; present and foreign meets the same 403 a foreign write meets — never a silent overwrite.
- **Patch** carries `givenName`, `familyName`, `status` (`active` ⇄ `suspended`, nothing else). `email` is immutable; no password field reaches this verb.
- **Archive** forces `status = suspended`. No unarchive, on the root or per entry.
- `user:update` reaches **neither** credential route: fixing a typo in a name must not become an account takeover.

### The two credential routes

| | `PATCH /:id/password` (change) | `PATCH /:id/password-reset` (reset) |
|---|---|---|
| Permission | `user:change-password` — the door every user needs | `user:reset-password` — the helpdesk |
| Self rule | must be self, else 403 | must NOT be self, else 403 |
| Current password | required and verified (Argon2id) | not carried — not knowing it is the point |
| must-change flag | **cleared** | **set** |
| Password policy | same value rules as the insert | same value rules as the insert |
| New = current | refused, from the two plaintexts | refused, against the stored hash |
| Answer | REST `204`, no body; GraphQL `success: true` | REST `204`, no body; GraphQL `success: true` |

**Self routes, it does not restrict.** Between the two doors every row is reachable: your own through the change, everybody else's through the reset. A `*:*` claim crosses the tenant scope on both — a super-admin sitting in a master tenant resets a password in any real tenant — and it does not cross either self rule, which costs it nothing: the change on somebody else's row would need their `currentPassword` anyway, and self-reset is the one thing the rule exists to refuse. Without it a stolen `user:reset-password` token would point at its own row and replace the credential without proving the previous one.

The row itself is read by `ScopedReader`, which scopes the REQUEST (ctx, deadline, trace) and not the tenant — `FindByID` queries by id alone. Tenant containment on these two routes is the aggregate's guard, not the read.

### What a user may be granted

Judged on the entries a write ATTACHES; a detach adds none and is judged by none. Same three questions per collection, one hop apart.

| Rule | `groups` | `roles` |
|---|---|---|
| available-in-tenant | The group must be in the table, active, and owned by the USER's tenant — one notification for all three, so it is no existence oracle over another tenant | Same, for the role |
| wildcard-refused | A group conferring any `*` permission cannot be joined through this API | A role granting any `*` permission cannot be granted |
| no-escalation | THREE hops: group → its roles → their permissions; the caller must hold every one | Two hops: role → its permissions |

Cap of 50 per collection, counted over the whole collection. The escalation gate stands down only when the request carried no identity at all — a dev bench.

---

## Client

Tenant-scoped: `clients.tenant_id`. The machine identity: two collections (`roles`, `allowedCIDRs`) and a hand-written secret rotation. The only entity whose rules ask what KIND of caller is on the token.

| Endpoint | GraphQL | Permission | Admission | Tenant scope | Self | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /clients` | `createClient` | `client:insert` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /clients/:id` | `patchClient` | `client:update` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /clients/:id/archive` | `archiveClient` | `client:archive` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `POST /clients/:id/roles` | `addClientRole` | `client:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /clients/:id/roles/:clientRoleId/archive` | `removeClientRole` | `client:grant` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `POST /clients/:id/allowedCIDRs` | `addClientAllowedCIDR` | `client:manage-network` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `PATCH /clients/:id/allowedCIDRs/:clientAllowedCIDRId/archive` | `removeClientAllowedCIDR` | `client:manage-network` | JWT + claim | guard foreign-tenant | — | yes | own tenant |
| `POST /clients/:id/secret` | `rotateClientSecret` | `client:rotate-secret` | JWT + claim | guard foreign-tenant | `kind:client → self` (dormant) | yes | own tenant |
| `GET /clients` | `clients` | `client:read` | JWT + claim | filter TenantID | — | yes | own tenant |
| `GET /clients/:id` | `client` | `client:read` | JWT + claim | filter TenantID | — | yes | own tenant |

**One row rule, and it is about MACHINES rather than ownership.** A user token of the tenant meets it on no route at all: it holds the permission, the row is in its tenant, and it writes — so an operator with `client:rotate-secret` rotates the secret of any client in their tenant, the mirror of User's reset. What it bounds is a CLIENT-subject caller, and only on the rotation.

**Everything else a client-subject caller may do, it may do to a sibling**: create, edit, archive, grant a role, edit the allow-list — ordinary tenant-scoped writes, gated by the permission the caller carries and nothing more. Until 2026-08-28 the rule covered every update and the archive, and a companion rule refused a client-subject insert outright; both were narrowed away as closed past the point of usefulness. Rotation is the exception because it is not editing a row: it mints a credential AND starts retiring the one in use, so a machine able to rotate another machine's secret could lock it out and take its place in one call.

**`dormant` means nothing reaches it yet**: it reads `RequestingIdentityKind`, fed from the `identity_kind` claim that only `POST /auth/client/token` mints — an endpoint that does not exist. Until it does the field reads `""`, the rule stands down, and that cell behaves as `—`. Nothing infers the kind from another claim's absence, deliberately.

- `client:rotate-secret` is a **sixth verb**, for the reason `user:reset-password` is its own: handing out a production credential is not editing a label.
- **Four verbs beyond the CRUD four**, one per job: `client:grant` (roles — confers privilege), `client:manage-network` (allowedCIDRs — decides WHERE FROM), `client:rotate-secret` (the credential). `client:update` reaches none of them; it carries `name`, `description`, `status` and nothing else.
- **An empty `allowedCIDRs` means ANY address** — `0.0.0.0/0` and `::/0` are refused so there is exactly one spelling of "no restriction". So archiving the last entry opens the credential to the whole internet, which is why the pair left `client:update` on 2026-08-28. Nothing enforces the list yet: it is read by no code until `POST /auth/client/token` exists.
- **Insert**: `tenantID` optional, absent means the claim's. `name` unique per tenant. **Patch** carries `name`, `description`, `status` (`active` ⇄ `suspended`); `tenantID` is immutable.
- **Archive** forces `status = suspended`. No unarchive, on the root or per entry.

### The secret

| | |
|---|---|
| Where the plaintext is readable | **The rotation response, and nowhere else.** The insert mints a credential and returns only `secretChangedAt` — so a usable secret comes from `POST /:id/secret`, including the first one |
| Rotation shape | OVERLAP, not swap: the old secret keeps working until `previousSecretExpiresAt` |
| `gracePeriodSeconds` | 0 to 604800, default 86400. **0 retires the old secret immediately** — the leaked case |
| Row state required | `active`. A suspended client is not handed a fresh credential |
| Answer | `200` with the plaintext (`rotateClientSecret` returns the same payload) — unlike User's two credential routes, which answer `204`, because those SET a credential the caller chose and this one MINTS one nobody can otherwise learn |

### What a client may be granted

Judged on the entries a write ADDS. The same three questions `User` asks of a direct role grant — and they bite harder here: a role granted to a person is exercised by a person who can be told no.

| Rule | Effect |
|---|---|
| role-available-in-tenant | The role must be in the table, active, and owned by the CLIENT's tenant — one notification for all three |
| no-wildcard-role | A role granting any `*` permission cannot be granted to a client through this API |
| no-escalation | The caller must hold every permission the role grants. `*:*` satisfies it by construction |

Caps: 50 roles, 20 CIDRs. A client caller may grant itself a role and gains nothing by it — the escalation rule bounds the grant to what it already holds.

---

## Implicit restrictions

Every line below is a request that **carries the permission** and is refused anyway. The `Permission` column in the tables above answers *who may attempt*; this section answers *what still does not pass*.

### By WHO is calling

A token of kind `user`, holding every permission:

| | id = you | id = somebody else |
|---|---|---|
| `PATCH /users/:id/password` (change) | ✅ | ❌ **403** |
| `PATCH /users/:id/password-reset` | ❌ **403** | ✅ |

A token of kind `client`, holding every permission *(dormant: no endpoint mints a client token yet)*:

| | id = itself | id = another row |
|---|---|---|
| `POST /clients/:id/secret` (rotate) | ✅ | ❌ **403** |
| `PATCH /users/:id/password` (change) | ❌ **403** | ❌ **403** |

The change refuses a client on both sides because the rule compares the token's subject against the `users` row id, and a client subject is never one.

### By WHAT is being written

| Request | Answer | Why |
|---|---|---|
| Grant a permission with `*` in either part to a role | **403** | No wildcard is grantable through this API, `*:*` callers included |
| Attach a role that grants a `*` permission to a group, a user or a client | **403** | Same rule one level up — which is why the platform's own superadmin group and user are not creatable here |
| Grant a role or permission the caller does not hold | **403** | No-escalation. A `*:*` claim satisfies every concrete permission, so it passes |
| Attach a role or group that belongs to another tenant | **422** | One notification for absent, archived and foreign alike — otherwise it is an existence oracle over another tenant |
| Change `email`, `key`, `workspace`, `resource`/`action`, or any `tenantID` | **422** | Immutable after creation; no request reaches them |
| Exceed a cap: 50 roles per group/user/client, 50 groups per user, 20 CIDRs per client | **422** | |
| Move a status anywhere but `active ⇄ suspended` | **422** | |

### By the STATE of the row

| Request | Answer | Why |
|---|---|---|
| Rotate the secret of a client that is not `active` | **422** | A suspended client was switched off deliberately; a fresh credential is the opposite of that |
| Create a group, a user or a client under an archived or `suspended` tenant | **422** | A `trial` tenant passes — unavailable is not the same question as not active |
| Unarchive anything but a tenant | **404** | No such route exists; archive is one-way everywhere else |

### By the TOKEN itself

| Condition | Effect |
|---|---|
| `must_change_password` is true | The token is signed carrying **only `user:change-password`**, `*:*` included. Different in kind from everything above: the permission does not travel at all, so the account may hold it while the session does not |

### And the scope nobody's permission crosses

Holding every concrete permission does not leave your tenant — only `IsSuperAdmin()` does, and it answers **false** for a resource wildcard like `user:*`. `*:*` is the one claim that crosses, and it still meets every refusal in this section except the no-escalation one.

---

## Public routes

No JWT, no claim, no permission.

| Endpoint |
|---|
| `GET /livez` |
| `GET /readyz` |
| `POST /auth/user/token` |
| `POST /auth/user/token/refresh` |

Every endpoint above answers on REST and on GraphQL alike — the generated ones by the
generator, and the three hand-written writes (the two user credential verbs and the client
secret rotation) by a mount written beside them on 2026-08-28, reusing the same handlers and
the same permissions. **Authentication is the only REST-only surface left**, and the four
public routes below are it.
