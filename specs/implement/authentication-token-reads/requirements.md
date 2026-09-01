# POST /auth/user/token — what the endpoint actually needs, and what it reads today

Working note for the read-path redesign. Written before any change, from the code,
so the redesign answers requirements rather than the shape that happens to exist.

## 1. Every datum the endpoint consumes, and who consumes it

| datum | consumed by | why |
|---|---|---|
| `users.password_hash` | `PasswordMatches` | the credential check |
| `users.status` | `accountIsUsable` | a suspended account is refused |
| `users.id` | subject of the token, `ResolveGrants` | identity |
| `users.email`, `given/family_name` | `email` / `name` claims, body | |
| `users.must_change_password` | claim, and it EMPTIES the custom claim set | |
| `users.tenant_id` | `tenant_id` claim, catalog lookup | |
| `tenants.workspace` | `tenant_workspace` claim, body | |
| `tenants.status` | `accountIsUsable` | `suspended` is refused; `trial`/`active` pass |
| `tenants.deleted_at` | — | depth: a row archived outside the aggregate |
| `user_claims.claim_id` → `value` | `heldClaimValues` | level 1 of the claim chain |
| claim catalog: `name`, `value_type`, `default_value` of the tenant's ACTIVE definitions | `resolveCustomClaims` | level 2, and the VOCABULARY the walk iterates |
| group keys (live groups only) | `groups` claim, body | |
| group names | body only | display |
| role keys, by any path (live roles only) | `roles` claim, body | |
| role names (direct grants only) | body only | display; inherited roles have none by design |
| permissions (live catalog entries only) | `permissions` claim, body | authorization across the mesh |

### What is NOT needed, though it is read today

- `user_claims → claims` join (`ClaimName`, `ClaimValueType`). `heldClaimValues`
  reads `entry.ClaimID` and `entry.Value` and nothing else; the name and the type
  come from the catalog, which is the anchor the walk iterates. Verified by
  grepping every field reference in `authentication_claims_manual.go`.
- The `User` aggregate's revision, and its write-side machinery. This is a read.

## 2. What it reads today: 8 statements

Captured from the Postgres statement log, one sign-in:

| # | statement | issued by |
|---|---|---|
| 1 | `users` ⋈ `tenants` | `FindUserByEmail` (root) |
| 2 | `user_groups` ⋈ `groups` | same load (child hydration) |
| 3 | `user_roles` ⋈ `roles` | same load (child hydration) |
| 4 | `user_claims` ⋈ `claims` | same load (child hydration) |
| 5 | `user_groups` ⋈ `groups` | `ResolveGrants` |
| 6 | `roles` + subqueries | `ResolveGrants` |
| 7 | `role_permissions` ⋈ `permissions` | `ResolveGrants` |
| 8 | `claims` of the tenant | `ClaimDefinitionsOfTenant` |

### The waste, named

- **#2 and #5 are the same read.** Same table, same join, same request. The only
  difference is that #5 also selects `groups.deleted_at` and filters on it. #2 is
  paid because the aggregate loader hydrates every declared child, whether the
  caller wants it or not.
- **#3 is paid and largely discarded.** It brings the DIRECT grants with role
  keys; #6 re-resolves the same roles (plus the inherited ones) because #3 carries
  no archive stamp for its target. Only the role NAMES survive from #3, and only
  for the response body.
- **#4 is paid with a join nothing in this flow reads.**

The root cause is one decision: **entering through the aggregate**. Three of the
four statements it costs exist because a `User` load hydrates three collections,
and this endpoint needs one of them (the claim values), without its join.

## 3. Constraints the redesign must not break

1. **The refusal is uniform.** Unknown address, wrong password, suspended account,
   archived tenant — one indistinguishable answer, and the timing equalised by
   `BurnPasswordVerification`.
2. **Absence vs. failure stays distinguishable in the LOG.** `(nil, nil)` means
   "provably not here"; an error means "nobody knows". The attempt row records
   them differently.
3. **Every gate already proven stays proven.** Archived: user, tenant, group,
   role, permission, claim definition. Revoked: grant, membership.
4. **A role that grants nothing is still a role the user holds.**
5. **Token claims are KEYS; display names live in the body only.**
6. **One resolution feeds both the token and the body** — they cannot disagree.
7. `must_change_password` empties the custom claim set and narrows permissions.

## 4. The redesign: 5 statements, 2 round trips, every access by index

Stop entering through the aggregate, and **anchor every read on the table the USER
indexes** — not on the catalog it points at.

| # | anchor | traversal | brings |
|---|---|---|---|
| A | `users` | ⋈ `tenants` | the account, workspace, tenant status; gated on a live tenant |
| B | `user_roles` | → `roles` → `role_permissions` (1:N) → `permissions` | roles held DIRECTLY, and what they confer |
| C | `user_groups` | → `groups` → `group_roles` (1:N) → `roles` → `role_permissions` (1:N) → `permissions` | the memberships, the roles they confer, and what THOSE confer |
| D | `user_claims` | — | `claim_id` → `value` (level 1) |
| E | `claims` | — | the tenant's ACTIVE definitions (level 2 + vocabulary) |

A is sequential — a sign-in arrives with an email and everything else is keyed on
the id it yields. B–E run concurrently. **Two round trips.**

### The 1:N traversal is a declaration, not a framework feature

A join renders `target.<ID> = anchor.<fk>` and the framework does not police what
that means. Giving a target schema `ID("role_id")` renders
`role_permissions.role_id = roles.id` — one role fanning out to its grants. That
is what puts the whole grant graph of a path in ONE statement, and what lets a
role that confers nothing survive it (LEFT join, nil grant).

### Why the anchor matters more than the statement count

The first attempt anchored B on `roles` and filtered `id IN (subquery)`. It reads
well and plans badly: Postgres turns the IN into a hashed SubPlan and SEQ SCANS
the catalog. On a tenant of 5k roles it discarded 5.003 rows to find 3, and the
whole read side went from 670µs to **1.658µs** — worse than the aggregate it
replaced. Re-anchoring on `user_roles` made every access an index scan.

### Why the memberships are not a sixth statement

C enters at `user_groups` and its first hop is the group itself, so the `groups`
claim falls out of a read that had to happen anyway.

### Why B and C are two and not one

Each path has its own index to enter by. A single read reaching both would have to
start above them — which is exactly the seq scan this shape exists to avoid.

## 5. Measured on the dev bench

Small fixture (7 roles), then a tenant of 5k roles / 2k permissions / 10k users:

| | small | at scale |
|---|---|---|
| the aggregate path (8 statements) | 1.582µs | — |
| anchored on `roles` + IN(subquery) | 670µs | 1.658µs |
| **anchored on the user's own index** | **~670µs** | **724µs** |
| an unknown address | 221µs | 256µs |
| one empty round trip | ~195µs | ~193µs |

The point of the last row: 724µs is under four empty round trips, and the shape
stops degrading with volume — which the two earlier shapes both did.

## 6. Shape of the code

The endpoint gets its OWN Direct schemas and repositories. It is not a CRUD read
and it is not an aggregate load: it is one very particular question, asked once
per sign-in, and the shapes it wants match no other caller in the service.

The port keeps TWO steps rather than collapsing into one, and the reason is the
attack path: the account read is what a wrong password is refused by, so the other
four must stay behind it. An unknown address costs one statement.
