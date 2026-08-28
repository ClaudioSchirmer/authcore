# Access Matrix

Who reaches what, per endpoint. The code as it stands.

Status: **in progress**. Mapped: Permission, Tenant, Role.
Pending: User, Group, Client, Authentication.

## Columns

| Column | Reads |
|---|---|
| **Permission** | What `RequirePermission` demands at the mount. Satisfied exactly, by resource wildcard (`x:*`), or by `*:*` |
| **JWT** | Bearer token required |
| **Tenant claim** | `tenant_id` claim required to pass admission — service-wide gate, 403 without it, no permission bypasses it |
| **Tenant scope** | What ties the row to the caller's tenant. `filter TenantID` = the read query forces `Filter["TenantID"] = id.TenantID()`. `guard foreign-tenant` = the domain refuses a write whose row is not the caller's (`TenantMismatchNotification`). `none` = nothing does |
| **`*:*` crosses** | Whether `IsSuperAdmin()` lifts that scope |
| **Rows reached** | What the caller ends up touching |

---

## Permission

`permissions` has no `tenant_id` column.

| Endpoint | GraphQL | Permission | JWT | Tenant claim | Tenant scope | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /permissions` | `createPermission` | `permission:insert` | yes | yes | none | n/a | creates |
| `PATCH /permissions/:id` | `patchPermission` | `permission:update` | yes | yes | none | n/a | any row |
| `PATCH /permissions/:id/archive` | `archivePermission` | `permission:archive` | yes | yes | none | n/a | any row |
| `GET /permissions` | `permissions` | `permission:read` | yes | yes | none | n/a | all rows |
| `GET /permissions/:id` | `permission` | `permission:read` | yes | yes | none | n/a | any row |

No unarchive mounted. Update carries `Description` only — `resource` and `action` are reachable by no request after creation.

---

## Tenant

The isolation partition itself. The row id is the `tenant_id` claim.

| Endpoint | GraphQL | Permission | JWT | Tenant claim | Tenant scope | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /tenants` | `createTenant` | `tenant:insert` | yes | yes | none | n/a | creates |
| `PATCH /tenants/:id` | `patchTenant` | `tenant:update` | yes | yes | **none** | n/a | **any tenant** |
| `PATCH /tenants/:id/archive` | `archiveTenant` | `tenant:archive` | yes | yes | **none** | n/a | **any tenant** |
| `PATCH /tenants/:id/unarchive` | `unarchiveTenant` | `tenant:archive` | yes | yes | **none** | n/a | **any tenant** |
| `GET /tenants` | `tenants` | `tenant:read` | yes | yes | **none** | n/a | **all tenants** |
| `GET /tenants/:id` | `tenant` | `tenant:read` | yes | yes | **none** | n/a | **any tenant** |

Both `ToCriteria` return the criteria unchanged; the four write commands bind no identity. Nothing compares the target row against the caller's `tenant_id` — containment is the distribution of `tenant:read` / `tenant:update` / `tenant:archive` and nothing else.

Archive and unarchive share `tenant:archive` — one permission, both directions. Archive also forces `Status` to `suspended`.

Update carries `Name`, `Description`, `Status` — `Workspace` is reachable by no request after creation.

---

## Role

Tenant-scoped: `roles.tenant_id`. Owns the `permissions` collection — the grants.

| Endpoint | GraphQL | Permission | JWT | Tenant claim | Tenant scope | `*:*` crosses | Rows reached |
|---|---|---|---|---|---|---|---|
| `POST /roles` | `createRole` | `role:insert` | yes | yes | guard foreign-tenant | yes | own tenant |
| `PATCH /roles/:id` | `patchRole` | `role:update` | yes | yes | guard foreign-tenant | yes | own tenant |
| `PATCH /roles/:id/archive` | `archiveRole` | `role:archive` | yes | yes | guard foreign-tenant | yes | own tenant |
| `POST /roles/:id/permissions` | `addRolePermission` | `role:update` | yes | yes | guard foreign-tenant | yes | own tenant |
| `PATCH /roles/:id/permissions/:rolePermissionId/archive` | `removeRolePermission` | `role:update` | yes | yes | guard foreign-tenant | yes | own tenant |
| `GET /roles` | `roles` | `role:read` | yes | yes | filter TenantID | yes | own tenant |
| `GET /roles/:id` | `role` | `role:read` | yes | yes | filter TenantID | yes | own tenant |

The two collection endpoints answer on BOTH surfaces, same command and same permission. The one difference is the shape, not the reach: on GraphQL the entry id rides the input (`rolePermissionId`) rather than a path segment, and the revoke resolves to `success: true` where REST answers `204`.

Granting and revoking are `role:update`, not permissions of their own.

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

## Public routes

No JWT, no claim, no permission.

| Endpoint |
|---|
| `GET /livez` |
| `GET /readyz` |
| `POST /auth/user/token` |
| `POST /auth/user/token/refresh` |

Every generated endpoint above answers on REST and on GraphQL alike. The HAND-WRITTEN routes
— authentication, the user credential verbs and the client secret rotation — are REST-only:
they are mounted outside the generator, so nothing puts them on the schema by default.
