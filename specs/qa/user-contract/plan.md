# QA plan — `user-contract`

- **Status:** APPROVED — maintainer (Cláudio Schirmer Guedes), 2026-09-07. *"bora, aprovado — toca ficha"* — the plan as a whole, including the six EDITED files of §5 and the four prose corrections of §0c.
- **Suite slug:** `user-contract` — what this round proves: the whole wire contract of the
  `User` aggregate and its **three** collections on both surfaces, the business rules its own
  specs and the maintainer state, and the four things that make this entity unlike every
  other one already proven:
  1. **it is the only aggregate whose WRITES decide who can hold a session** — `U15` forces
     `suspended` on archive and `AccountIsUsable` refuses a non-active account and a
     commercially suspended tenant, with **one generic answer for both**;
  2. **it carries a credential**, and with it a `RedactedField` nothing else in the service
     has, three doors that must judge a password identically, and two routes that refuse
     each other's rows;
  3. **a token minted for a `must_change_password` account carries exactly one permission**,
     `*:*` included — an invariant that lives in no entity spec and that no route
     declaration reveals;
  4. **both flavors of the dual 409 are reachable in one lane** — `SemanticConflict` from
     four duplicate paths and `SemanticStateConflict` from `InvalidUserStatusTransition`.
     `Client` is the only other aggregate carrying a `SemanticStateConflict` notification
     (`ClientMustBeActiveToRotate`) and it has no lane yet, so this is the first round in
     which the split is exercised at all — and the asymmetry worth knowing is with `Tenant`,
     whose `InvalidTenantStatusTransitionNotification` declares **no** `Semantic()` and is
     therefore a **422**: two status machines, two different status codes.
- **Written:** 2026-09-07
- **Pin:** omnicore **`v0.74.0`** · dialect postgres · read backing **relational**
  (read-your-writes — every read-back below is IMMEDIATE; a needed poll would itself be a
  failure)
- **Surfaces in scope:** REST + GraphQL (`surfaces.graphql.enabled: true`) — **full parity,
  14 operations each**, and *not* the "root verbs only" `spec.md` §9 still describes; see §0c
- **Plan destination:** `specs/qa/user-contract/plan.md` · **suite destination:** `qa/` at the
  project root · **verdict destination:** `qa/qa-report.md`
- **Prior rounds:** `specs/qa/tenant-contract/plan.md` (APPROVED 2026-09-06),
  `specs/qa/permission-contract/plan.md`, `specs/qa/role-contract/plan.md` and
  `specs/qa/group-contract/plan.md` (all APPROVED 2026-09-07). This round **EXTENDS the same
  `qa/run.sh`** — no second entry point — and none of the four approved plans is reopened.

## 0. Where every expectation below comes from

Nothing here was read off a running service. The service was never called while this plan was
written. The one exception is an ENUMERATION and never a value that becomes an expectation:
the suite reads `GET /openapi.json` at boot to cross-check the verb inventory (`M11`).

| Source | What it settled |
|---|---|
| `specs/scaffold-entity/user/spec.md` (APPROVED 2026-08-25, 1927 lines) | the model; §B Q0–Q6; §5 the one-way archive and the e-mail index that justifies it; §6 soft-delete only; §7 the U0–U16 rule table; §9 the read vocabulary, the 1:N boundary and the `passwordHash` filter policy; §10 the seven permissions and the three authorization layers |
| `specs/evolve-entity/user/spec.md` (APPROVED 2026-08-28) | the `claims` collection: §4a the shape, §4d the cap of **20** and the three manual rules with their ORDER and interlock, §4e the `user:set-claim` verb, and the 2026-08-28 correction that brought the `change` verb back as a PATCH |
| `specs/omnicore-gen/user.omnicore.yaml` | fields, the three collections, the four joins, notifications, modes, `patchExcludes`, rules (declarative + manual) and their order, `service.facts`, `read.computed`, `authz` |
| `internal/domain/user.go` | `Modes()` = display/insert/update/archive (**no unarchive**); the three caps; the e-mail pre-check; the status transition map `active↔suspended`; `refuseForeignTenant` under `IfInsertOrUpdate` **and** `IfArchive`; the three `Add…`/`Remove…ByID` collision paths and `ChangeUserClaimByID` |
| `internal/domain/user_rules_manual.go` | `deriveCredential` (hash + `MustChangePassword = true` at birth + `EmailVerifiedAt` left NIL); `refusePasswordEchoingIdentity` and its 4-rune floor; `refuseUnavailableTenant` (trial PASSES); the three ADDED-entries walks and the `continue` interlock behind each |
| `internal/domain/user_credential_manual.go` | `ActionChangePassword` / `ActionResetPassword` as the discriminators; `rowIsTheCaller` reading `Subject` and not `Claims["sub"]`; the two opposite row decisions; `credentialValueRules` calling `IsValid` directly **because `ValidateValueObject` loses to the generated `IgnoreValueObject` in silence**; `applyNewCredential` and the one thing the two operations disagree about |
| `internal/application/commands/handlers/utils/authentication.go` | `AccountIsUsable` (non-active account **and** suspended tenant, trial passes) · `EffectivePermissions` → `restrictToPasswordChange`, the **embedded** single grant · `BuildClaims` / `BuildProfile` reading ONE resolution so body and token cannot disagree — the source that makes §1b `U23` provable rather than hopeful |
| `internal/infra/schemas/user_schema.go` | the `Composite(PersonName)` exposed as `GivenName`/`FamilyName`, and **`RedactedField("PasswordHash", InSync ***, InAudit ***)`** — the only one in the service |
| `internal/infra/user_repository.go` | the five constraint bindings and the **four** read joins: one root `InnerJoin` into Tenant and three `InnerJoinInChild`, all with their counterpart's `archived_at` SERVED |
| `internal/web/user_routes.go` · `internal/web/user_credential_routes_manual.go` | the **14** REST endpoints and the **14** GraphQL fields, and the permission literal on each — including the two credential routes that §9/§10 say are REST-only and are not |
| `internal/web/requests/find_users_by_params.go` · `find_user_by_id.go` · `insert_user.go` · `patch_user.go` · the six collection requests · `dtos/user_{group,role,claim}.go` · `change_password.go` · `reset_password.go` | the DTO opt-in gate: which controls each read serves (**no `search`**), which operators each leaf admits, and which fields each body carries — including that **no Response DTO declares `PasswordHash`** while both query Results do |
| `internal/application/queries/find_users_by_params_query.go:…ToCriteria` · `find_user_by_id_query.go:…ToCriteria` | `Filter["TenantID"] = id.TenantID()` unless `IsSuperAdmin()` — Layer 3, and why a foreign row answers **404** and not 403 |
| `migrations/postgres/0005_user_manual.up.sql` · `0010_user_claims_manual.up.sql` | the four partial unique indexes (`email`; and `(user_id, X)` per collection, all `WHERE archived_at IS NULL`), and the four hand-appended cross-aggregate FKs that make every INNER join safe |
| `migrations/postgres/0012_bootstrap_seed_manual.up.sql` | the token source, the eight `user:*` catalog rows (`…001f`–`…0026`) and the four `claim:*` rows (`…0001`–`…0004`) |
| `qa/microservice.qa.yaml` · `qa/run.sh` · `qa/lib/common.sh` | the posture this suite runs under and the helper API every new lane speaks |
| pin docs — `status-mapping`, `auto-handlers`, `auto-query-handlers`, `read-joins`, `relational-view`, `auth-middleware`, `authz-seams`, `graphql`, `rules-dsl`, `audit`, `token-issuance` | every status code, notification key and envelope shape asserted below |
| pin source — `application/notifications/core.go:397-421` | `TenantMissing` and `TenantMismatch` are both `SemanticForbidden` → **403** |
| **the maintainer, asked 2026-09-07** (`AskUserQuestion`, two rounds) | §0c's disposition; §1b `U23` in its full five-case form; §1b `U24`'s three doors both ways; `U25`'s four faces; the six escalation rows of `U13`/`U14`; the HYBRID cap form of `U15`; the four credential families; and `M6` including the e-mail reissue |

### 0a. What this round INHERITS and does not repeat

Proven service-wide by the four approved rounds, against this exact posture:

- the whole **401 family** (`S3a.1`–`S3a.10`) — token shape, signature, `iss`, `aud`, `alg`,
  `exp`, and the expired-vs-invalid key split;
- the **framework's appended public surfaces** (`/docs`, `/openapi.json`, JWKS, the
  playground, the root redirect) and the **GraphQL introspection bypass with its four edges**;
- **both directions** of the public-route split and the exactness neighbours;
- the middleware **tenant-claim gate** (`authorization.tenant.required: true`);
- the **boot, hygiene and report contracts** (§2, §5, §6 of `tenant-contract`), reused
  verbatim;
- the generic **relational read contract** — cursor walks, the only-total conflict matrix,
  the page ceiling, the malformed-cursor family — asserted here only where `User`'s own
  vocabulary differs (`M7`, `M8`).

### 0b. What is DELIBERATELY not in this round

- **`Client` and `Claim` have no lane yet.** `Claim` is touched here only as the fixture the
  `claims` collection points at; its own contract is a later round. `Client` is untouched.
- **`/auth/user/token` is exercised, not owned.** `U23` and `U24` call it because they cannot
  be proven otherwise, and they assert only what the User aggregate decides. The token
  route's own contract — refresh, TTL, JWKS rotation — is not in this matrix.

### 0c. Stale prose in the User specs — three items, and the maintainer's disposition

All three are verifiable in the code and all three contradict something this suite asserts.
**Maintainer, 2026-09-07: "Corrigir os três agora, com nota de supersessão datada."** The
edits are listed in §5 and are applied as part of this round; the plan cites the corrected
text.

| # | What `spec.md` says | What the code does | Where this suite pins it |
|---|---|---|---|
| a | §9: "GraphQL: yes, **root verbs only** (`users`, `user`, `createUser`, `patchUser`, `archiveUser`) — the four child operations and both password operations are REST-only"; §10 repeats it: "**Neither is on GraphQL.**" | `MountUsersGraphQL` registers **12** fields and `MountUserCredentialsGraphQL` registers **2** more. Full parity: 14 REST operations, 14 GraphQL fields, the same handler and the same permission on each | `N1`, `N5`, `N6` |
| b | §9: the computed read field is "**one — `name`**", with "`?fields=name`" and "`?orderBy=name` is a typed 400" | the field is `fullName` (`computed:"GivenName,FamilyName"`). `name` resolves to nothing on this read model | `M7.9`, `M8.12` |
| c | §10: "**Neither profile configures `auth.authorization` today** … so `RequirePermission` currently no-ops across the whole service and everything in this section is generated, correct and **inert**" | `microservice.dev.yaml` and `qa/microservice.qa.yaml` both carry `authorization.enabled: true` with `tenant.enabled/required: true`. The gate is live, and §3 below is what proves it | all of §3 |

A fourth item is a **wording** difference rather than a contradiction and is corrected in the
same pass: §9 says `hidden` keeps the hash out of every surface. The mechanism is two
distinct ones — the Response DTOs simply do not declare the field (which is what keeps it off
the wire) and `RedactedField(InSync/InAudit)` is what keeps it out of the framework's own
copies. §9's own conclusion is unchanged and its warning about the filter policy is exactly
right; only the mechanism named is.

---

## 1. Coverage matrix — the FRAMEWORK's promises

Entity **User** × surfaces **REST** (`qa/user.sh`, family **M**) and **GraphQL**
(`qa/user_graphql.sh`, family **N**). Read backing **relational**, so every write→read-back is
immediate. Archive regime *kept-but-hidden* (no `DeleteOnArchive`). The aggregate carries
**three** collections and **four** read joins.

Envelope asserted on REST: `errors[].messages[].notificationKey` + the HTTP status.
Envelope asserted on GraphQL: HTTP is **always 200**, and the same key rides
`errors[].extensions.notificationKey`. Every request pins `Accept-Language: en-US`.

The lane runs as the bootstrap admin (`*:*`), so nothing in §1 is blocked by row scope; §1b
and §3 are where the scoped principals do the work.

### M1 — Happy path, one per served verb (14 routes)

| REST | GraphQL twin | Expected |
|---|---|---|
| `POST /users` | `createUser(input:)` | **201** · the record AS STORED |
| `PATCH /users/{id}` | `patchUser(id:, input:)` | **200** · the record after the change |
| `PATCH /users/{id}/archive` | `archiveUser(id:)` | **204, NO BODY** / payload `{success}` |
| `GET /users` | `users(...)` | **200** · `data` + `pagination` / the Relay connection |
| `GET /users/{id}` | `user(id:)` | **200** · the full document |
| `POST /users/{id}/groups` | `addUserGroup(id:, input:)` | **201** · `{userId, userGroup:{id, groupID}}` |
| `PATCH /users/{id}/groups/{gid}/archive` | `archiveUserGroup(id:, input:)` | **204, NO BODY** / `{success}` |
| `POST /users/{id}/roles` | `addUserRole(id:, input:)` | **201** · `{userId, userRole:{id, roleID}}` |
| `PATCH /users/{id}/roles/{rid}/archive` | `archiveUserRole(id:, input:)` | **204** / `{success}` |
| `POST /users/{id}/claims` | `addUserClaim(id:, input:)` | **201** · `{userId, userClaim:{id, claimID, value}}` |
| `PATCH /users/{id}/claims/{cid}` | `patchUserClaim(id:, input:)` | **200** · the entry, id KEPT |
| `PATCH /users/{id}/claims/{cid}/archive` | `archiveUserClaim(id:, input:)` | **204** / `{success}` |
| `PATCH /users/{id}/password` | `changeUserPassword(id:, input:)` | **204, NO BODY** / `{success}` |
| `PATCH /users/{id}/password-reset` | `resetUserPassword(id:, input:)` | **204, NO BODY** / `{success}` |

`M1.15` — **`patchUserClaim` keeps the entry's id.** The whole point of the verb the
2026-08-28 correction brought back: the read-back reports the SAME child id with the new
value, never a new id, and never an id plus a removal.

### M2 — The write/read asymmetry, asserted as a contract

`M2.1` A collection write response carries **the stored column and nothing traversed**: each
entry is exactly `{id, groupID}` / `{id, roleID}` / `{id, claimID, value}` — no `groupKey`,
no `roleName`, no `claimValueType`, no `*ArchivedAt`. `M2.2` A READ response carries the
traversal: five keys per group entry, five per role entry, six per claim entry.
`M2.3` `POST /users` answers a body with **no `createdAt`/`updatedAt`/`archivedAt` and no
`tenantWorkspace`/`tenantStatus`/`tenantArchivedAt`** — the insert Response declares none of
the six, and a suite that assumed symmetry with the read would assert a field the contract
never promised. `M2.4` The same for `PATCH /users/{id}`.

### M3 — Golden record: every declared field, written then read back

One user exercising every field of the model, read back field-by-field on **both** surfaces.

- **The composite is spoken as its EXPOSED PARTS.** `givenName` and `familyName` are the
  only names any surface carries; the composite's own field name (`Name`) appears **nowhere**
  — not in a body, not in a filter, not in `?fields=`, not in the GraphQL schema.
- **`fullName` is served ready** on both reads, equal to `givenName + " " + familyName`.
- **`emailVerifiedAt` is `null` on a freshly created user** — `deriveCredential` leaves it
  NIL deliberately, because nothing in this service verifies an address.
- **`mustChangePassword` is `true` on a freshly created user**, always: an admin-set initial
  password has to be replaced by its owner.
- **`passwordChangedAt` is present and non-null** on every user, since a password is
  required at creation.
- **`passwordHash` is ABSENT** from every one of the six bodies on both surfaces — absent,
  not present-and-null (`assert_absent`, the `has()` distinction the permission round
  established). This is `U25`'s first face and it is asserted here as well because the golden
  record is the family that catches a field silently ADDED to a DTO.
- **The three joined stamps ride the read**: `tenantWorkspace`, `tenantStatus`,
  `tenantArchivedAt` on the root; `groupKey`/`groupName`/`groupArchivedAt`,
  `roleKey`/`roleName`/`roleArchivedAt`, `claimName`/`claimValueType`/`claimArchivedAt` per
  entry.

### M4 — Validation, 422, asserting the KEY

One representative failure per value object and per declarative shape:
`InvalidPersonNameNotification` (under `Given` and under `Family` — the composite reports per
part) · `InvalidEmailNotification` · `WeakPasswordNotification` (each of the four classes and
both length bounds, plus leading/trailing whitespace) · `UnknownUserStatusNotification` ·
`RequiredFieldNotification` on an empty `email`/`password`/name half ·
`InvalidIDUUIDNotification` on a malformed `tenantID` — and, because `U0` declares
`TenantID` a **guard**, a request carrying a malformed `tenantID` **and** a second problem
reports the id alone: the pass ENDS there.

### M5 — The dual 409, and User is where both flavors live

**Duplicate flavor (`SemanticConflict`), four paths:**
`UserEmailAlreadyExistsNotification` (the service pre-check, and the partial unique index as
the concurrent backstop) · `UserAlreadyInGroupNotification` ·
`UserAlreadyGrantsRoleNotification` · `UserAlreadyHoldsClaimNotification`. Each asserted
twice — once through the domain's `IsSameBusinessIdentity` walk on a second `POST`, and once
through the collection's `insertOrUpdate` path in a single body carrying the same id twice.

**Wrong-state flavor (`SemanticStateConflict`), one path — and the first one any lane in
this suite has exercised:**
`InvalidUserStatusTransitionNotification` → **409**, semantic string `"StateConflict"`, on a
`PATCH` moving `status` to a value the map does not allow. `M5.6` asserts the two allowed
edges pass (`active→suspended`, `suspended→active`) and `M5.7` that a **no-op** passes — a
`PATCH` re-sending the current value is always legal, which is what keeps an unrelated
rename from being hostage to the state machine.

### M6 — Archive, and the address it releases

`M6.1` `PATCH /users/{id}/archive` → **204**. `M6.2` the row vanishes from the listing.
`M6.3` `?includeArchived=true` reveals it, with `archivedAt` non-null. `M6.4` the by-id read
without the flag answers **404**; with it, **200**. `M6.5` **`PATCH /users/{id}/unarchive`
→ 404 `RouteNotFoundNotification`** — nothing is registered at that path, which is a 404 and
not a 405; the same shape the other three rounds assert, because **`Tenant` is the only
aggregate in this service that mounts an unarchive at all**.

`M6.6` **the address comes back.** `POST /users` with the EXACT e-mail of the archived user
answers **201** with a new id. This is the case that proves `spec.md` §5's reasoning has a
basis: `users_email_key` is `WHERE archived_at IS NULL`, so archiving releases the handle —
and that release is precisely why no unarchive could be mounted here.

`M6.7` **the root archive cascades onto its active entries**, all three collections: every
`user_groups` / `user_roles` / `user_claims` row of that user is stamped (asserted through
SQL, the same shape `K6.11b` uses). `M6.8` **the stamp is scoped**: an entry archived on its
own BEFORE the root's archive carries its ORIGINAL stamp, distinct from the root's — the
cascade does not restamp what was already out.

`M6.9` writes onto an archived user answer **404** (`LoadForWrite` runs `ScopeActive`), one
case per collection verb and one for the credential pair.

### M7 — Read vocabulary, per declared operator family

Filters, one representative per family, per leaf, from the `filter:` tags on
`FindUsersRequest`: `tenantID` (`eq,in`) · `givenName` (`eq,in,startswith,istartswith,contains,icontains`) ·
`familyName` (+`ne`) · `email` (+`ne`) · `status` (`eq,in`) · `mustChangePassword` (`eq`) ·
`passwordChangedAt` (`gte,lte`) · `emailVerifiedAt` (`eq,gte,lte`) · `createdAt`/`updatedAt`
(`gte,lte`) · `id` (`eq,in`) · **and the two ROOT-JOIN leaves, `tenantWorkspace` and
`tenantStatus`** — the reach that distinguishes this backing, asserted as a 200 that returns
the right rows.

`M7.9` `?fields=fullName` → **200**, the computed path fetching its two sources.
`M7.10` `?fields=groups.groupKey,roles.roleKey,claims.claimName` → **200**: a child join's
fields ARE addressable in a projection.
`M7.11` `?onlyTotal=true`, alone and beside a filter. `M7.12` `?includeArchived`.
`M7.13` the pagination envelope as a contract — `totalCount`/`hasNextPage`/`hasPreviousPage`/
`startCursor`/`endCursor` truthfulness, page-2 disjointness, a forward walk echoing
`endCursor` into `?after=`, and `?last=` alone serving the TAIL window.

### M8 — Rejected reads: the whole typed-400 family, and the password oracle

`M8.1` unknown filter key · `M8.2` an operator outside a leaf's allowlist (`?status.contains=`) ·
`M8.3` **`?search=`** — undeclared on this DTO → **400 `SchemaViolationNotification`**, the
opt-in gate reached before any engine, and `spec.md` §9's recorded decision ·
`M8.4` an undeclared reserved control, PRESENCE tripping it ·
`M8.5` an unresolvable `?fields=` path, asserting `field: fields[bogus]` ·
`M8.6` a filter VALUE outside the leaf's kind (`?emailVerifiedAt.gte=lixo`,
`?tenantID.eq=lixo`) → **400 `InvalidFilterValueNotification`** ·
`M8.7` `?first=` above the ceiling · `M8.8` mixed directions · `M8.9` `?onlyTotal=true`
beside a page-shaping control · `M8.10` malformed cursor · `M8.11` cursor↔`orderBy` and
cursor↔`includeArchived` mismatch ·
`M8.12` **`?orderBy=fullName`** → 400 on `orderBy[fullName]` — a computed path backs no
column (and `?orderBy=name` is a 400 too, for the different reason that no such field exists:
§0c item b).

**The 1:N boundary, asserted from both sides.** `M8.13` `?groups.groupKey.eq=engineering`,
`?roles.roleKey.eq=…` and `?claims.claimName.eq=…` are each a **400** — a served field is
not thereby an addressable one — while `M7.10` above showed the same paths ARE projectable.
"Who is in Engineering?" is not answerable from this listing, which is `spec.md` §9's stated
cost pinned where it will be rediscovered.

**The password oracle, `U25`'s second and third faces, asserted here as read contract.**
`M8.14` `?passwordHash.eq=$argon2id$v=19$m=19456,t=2,p=1$AAAA` → **400
`SchemaViolationNotification`** · `M8.15` `?passwordHash.startswith=$a` → 400 ·
`M8.16` `?orderBy=passwordHash` → 400 · `M8.17` `?fields=passwordHash` → **400**, never a
silent `200 {}` — the same doctrine `E7.8` approved for a stored-and-not-selectable field,
and it matters double here: a `200` with the key merely absent would leave open whether the
column had been read.

These four are the ONLY thing standing between a hash and a character-at-a-time walk. `spec.md`
§9 says so in as many words: the framework "offers the mechanism and does not decide the
policy", and the policy is the ABSENCE of a `filter:` tag. **These cases fail on the day
somebody adds one**, which is the entire reason they exist.

### M9 — Absent verbs, wrong addresses, malformed ids

`M9.1` no route at all (`PATCH /users/{id}/unarchive`, `DELETE /users/{id}`) → **404
`RouteNotFoundNotification`** · `M9.2` a mounted path under another method → **405** ·
`M9.3` a valid-but-absent uuid → **404** on each read and each write ·
`M9.4` **a by-id address that is not a uuid, split by VERB**: `GET /users/lixo` → **404
`UnknownIDAddressNotification`**, `PATCH /users/lixo` → **400 `MalformedIDNotification`**,
and the same split on the child address (`…/groups/lixo/archive`). Identical on both
surfaces. `M9.5` a child id that is a valid uuid but belongs to another user → **404**:
the entry is addressed inside its owner, never globally.

### M10 — The four read joins, and what each half promises

| join | kind | fields | served? | filterable? | projectable? |
|---|---|---|---|---|---|
| Tenant | root `InnerJoin` on `tenant_id` | `tenantWorkspace`, `tenantStatus`, `tenantArchivedAt` | yes | **yes** (`M7`) | yes |
| Group | `InnerJoinInChild` on `group_id` | `groupKey`, `groupName`, `groupArchivedAt` | yes | **no** (`M8.13`) | **yes** (`M7.10`) |
| Role | `InnerJoinInChild` on `role_id` | `roleKey`, `roleName`, `roleArchivedAt` | yes | no | yes |
| Claim | `InnerJoinInChild` on `claim_id` | `claimName`, `claimValueType`, `claimArchivedAt` | yes | no | yes |

`M10.5` **the counterpart's archive stamp is the entry's own account of outliving what it
points at**: archive a Group that a live user still belongs to, and the user's read still
carries that entry, now with `groupArchivedAt` non-null and `groupKey`/`groupName` still
resolved. The INNER join matches on the id, not on the stamp, so the entry survives — which
is exactly what the served stamp is for. One case per collection.

### M11 — Route inventory cross-check

`GET /openapi.json` enumerates **14** `/users…` operations, each carrying the permission
literal `user_routes.go` and `user_credential_routes_manual.go` declare. An enumeration, never
a value that becomes an expectation; a source-vs-openapi disagreement is a FINDING.

### N — GraphQL, the same operations under the surface's own idiom

`N1` the 14 fields resolve · `N2` the golden record round-trips identically, `passwordHash`
absent from every selection · `N3` a typed refusal rides `errors[].extensions.notificationKey`
with HTTP **200** · `N4` an undeclared argument and an out-of-schema enum value are cut by
`gqlparser` BEFORE any resolver and carry **no** `notificationKey` — the surface's own idiom,
asserted as such and never as the REST envelope · `N5` **`changeUserPassword` and
`resetUserPassword` exist and answer `{success}`**, the two fields §0c item (a) says are not
there · `N6` the three collection mutation triples exist, `patchUserClaim` included, and
answer the same keys their REST twins do · `N7` `fields`/`onlyTotal` are selection-natural
here and are never gated.

---

## 1b. Domain expectations — from the specs, and from the maintainer

Rows ranked by the cost the maintainer named. These land in `qa/domain.sh` as family **U**,
beside `T`/`P`/`RL`/`GR`. Every row has BOTH a positive and a negative case; a row with only a
happy path is a rule nobody tested.

| # | The rule | Source | POSITIVE | NEGATIVE → status · key |
|---|---|---|---|---|
| **U23** | **A token minted for a `must_change_password` account carries EXACTLY `user:change-password` and nothing else — the grant is EMBEDDED, and `*:*` is dropped** | `authentication.go` `EffectivePermissions`/`restrictToPasswordChange`; **asked — maintainer, 2026-09-07, full chain** | after rotating, the next token carries the whole bundle and `GET /users` answers 200 | the pre-rotation token's `permissions` claim is exactly `["user:change-password"]`; `GET /users` with it → **403** |
| **U24** | **Who may hold a session: the account must be `active` AND its tenant must not be suspended — one generic answer for both** | `AccountIsUsable`; §7 `U15`; **asked — three doors, both ways** | an active user in an active (or **trial**) tenant signs in | suspended user → refused; archived user → refused; user of a suspended tenant → refused; **all three return the same key**, no oracle for which half failed |
| **U17** | **Archiving forces `suspended` — a mutation, not a validation** | §7 `U15`, DECIDED at the gate | archive answers 204 and raises nothing | `?includeArchived=true` shows `status: "suspended"` on a user archived while `active` — the rule reached the ROW, not just the event; and the audit row records it |
| **U19** | **The two credential routes refuse each other's rows** | `user_credential_manual.go`; §10; **asked** | change on one's own id → 204; reset on another's id → 204 | change pointed at another → **403 `PasswordChangeRequiresSelfNotification`**; reset pointed at self → **403 `PasswordResetRequiresAnotherUserNotification`** |
| **U20** | **The change CLEARS `mustChangePassword`; the reset SETS it** | `applyNewCredential`, the one thing the two disagree about; **asked** | after a change, the read-back reports `false` | after a reset, the read-back reports `true` — and the next token that account gets is `U23`'s restricted one |
| **U25** | **The password hash reaches no surface, no filter, no ordering, no projection and no audit payload** | `user_schema.go` `RedactedField`; §9's filter policy; **asked — four faces** | the hash verifies a login, so it IS stored | absent from all six bodies on both surfaces (`M3`); 400 on filter/orderBy/fields (`M8.14–17`); **`audit_events.payload` carries `***`** and the real hash string appears nowhere in the row (SQL) |
| **U13** | **No escalation into a GROUP — three hops (group→roles→permissions) — and no wildcard-bearing group** | §7 `U13a`/`U13b`; `refuseUnjoinableGroups` and its load-bearing order; **asked — all six rows** | principal **I** joins a user to a group whose roles confer only what I holds → 201 | I joins a group conferring `permission:archive`, which I lacks → **403 `CannotJoinGroupWithUnheldPermissionsNotification`**; a group carrying the `*:*` `master` role → **403 `CannotJoinWildcardGroupNotification`**, and it fires FIRST — the order that keeps `HasPermission` from panicking into a 500 |
| **U14** | **No escalation into a ROLE — two hops — and no wildcard-bearing role** | §7 `U12a`/`U12b`; `refuseUngrantableRoles` | I grants a role conferring only what I holds → 201 | a role conferring `permission:archive` → **403 `CannotGrantRoleWithUnheldPermissionsNotification`**; the `master` role → **403 `CannotGrantWildcardRoleNotification`**. The keys DIFFER from `U13`'s, which is what makes the depth of a refusal readable |
| **U8/U9/U10** | **A group / role / claim must exist, be active, and belong to THIS USER's tenant — one answer for all three causes** | §7 `U8`/`U9`; evolve §4d 1 | each attaches inside the tenant → 201 | an id from ANOTHER tenant, an ARCHIVED one, and an absent-but-well-formed uuid all answer the SAME key — `GroupNotAvailableInTenant` / `RoleNotAvailableInTenant` / `ClaimNotAvailableInTenant`, **422** — so nothing confirms a competitor's row exists |
| **U11** | **A claim whose `appliesTo` is `client` cannot be set on a user; `user` and `both` pass** | evolve §4d 2 — "what finally makes `AppliesTo` mean something" | `appliesTo: user` and `appliesTo: both` → 201 | `appliesTo: client` → **422 `ClaimDoesNotApplyToUserNotification`** |
| **U12** | **A claim value must parse as the definition's declared `valueType`** | evolve §4d 3, the same three readings the catalog's own default-value check uses | `number` ← `"1000"`; `bool` ← `"true"`/`"false"`; `string` ← any non-empty | `number` ← `"abc"`, `bool` ← `"yes"`, `string` ← `""` → **422 `ClaimValueDoesNotMatchValueTypeNotification`**, echoing the VALUE and not the id. **And the PATCH is judged too** — `refuseUnsettableClaims` walks ADDED *and* CHANGED, so correcting an entry to a bad value is refused exactly as adding one is |
| **U12b** | **The availability interlock: one bad id gets ONE answer** | evolve §4d's own correction to itself | — | an unresolvable claim id reports `ClaimNotAvailableInTenant` **alone** — not three notifications. Same for a group and a role id |
| **U5/U6/U7** | **The password policy holds at all THREE doors: insert, change and reset** | `credentialValueRules`, and the comment recording that the obvious repair fails in SILENCE and once let a 5-character password through; **asked** | a compliant password passes each door | per door: a short password → **422 `WeakPasswordNotification`**; a mismatched confirmation → **422 `PasswordConfirmationMismatchNotification`**; a password containing the e-mail's local part or a ≥4-rune name word → **422 `PasswordEchoesIdentityNotification`**, with the plaintext NOT echoed back |
| **U6b** | **The 4-rune floor is what keeps the identity rule usable** | `passwordIdentityWordMinRunes`; the "Ng" argument | a user named `Li Ng` may use a password containing `ng` → 201 | a user named `Maria Souza` may not use one containing `maria` → 422 |
| **U21** | **The new password must differ from the current one — on both routes, by different means** | §7 `U7b`; the change answers from two plaintexts, the reset from a full Argon2id verification | a different password → 204 | the same password → **422 `PasswordUnchangedNotification`** on the change AND on the reset |
| **U22** | **The change verifies the current password, and an EMPTY one is refused without hashing** | `changePasswordRules` | the right current password → 204 | a wrong one → **422 `InvalidCurrentPasswordNotification`**; an empty one → the SAME key |
| **U1** | **`email` is immutable after creation** | §7 `U2b` | a PATCH not carrying it → 200 | `PATCH` with a different `email`… **see the note below** |
| **U2** | **`tenantID` is immutable after creation** | §7 `U1` | — | the same note applies |
| **U3** | **The owning tenant must exist, be unarchived and not commercially suspended — a `trial` tenant PASSES** | §7 `U3`; `refuseUnavailableTenant`, `IfInsert` only | a user in an `active` tenant → 201; **in a `trial` tenant → 201**, the case that would break every trial signup if the rule read `Status != active` | an archived tenant, and a `suspended` one → **422 `UserTenantDoesNotExistNotification`**. And the deliberate asymmetry: **a PATCH still succeeds** on a user whose tenant has since been suspended — suspension withholds NEW users, it does not freeze the ones already there |
| **U4** | **Tenant isolation on WRITES, including the archive** | §7 `U4`; `refuseForeignTenant` under `IfInsertOrUpdate` AND `IfArchive` | a scoped principal writes inside its own tenant | creating a user with another tenant's id → **403 `TenantMismatchNotification`**; and the archive path refuses too, which is the gate a suite testing only inserts would miss |
| **U15** | **The caps: 20 claims, 50 groups, 50 roles — HYBRID form** (maintainer, 2026-09-07) | §7 `U11a`/`U11b`; evolve §4d `claims-cap` | **20 claims attach and the 20th succeeds** | the **21st** → **422 `TooManyClaimsForUserNotification`** (`max: "20"`), proven end to end. The two 50-caps are proven at the BOUNDARY only — one insert carrying 51 entries → `TooManyGroupsForUserNotification` / `TooManyRolesForUserNotification` — and this plan records that **the passing side at exactly 50 was not exercised** |
| **U16** | **Status transitions: `active↔suspended` and no-ops only** | §7 `U14` | both edges, and a no-op | any other move → **409 `InvalidUserStatusTransitionNotification`** (`M5`'s StateConflict flavor) |
| **U18** | **READ scope: a tenant JWT sees only its own tenant's users, and a foreign row is 404, not 403** | §7 `U16`; both `ToCriteria`s | a scoped principal reads its own tenant's users; a `*:*` holder reads every tenant's | the listing simply does not contain the foreign row (**an isolation leak is a 200**, which is why this needs its own case); the by-id read of a foreign user → **404** |

**Rows left UNPROVEN, named rather than quietly asserted:**

- **`U1` / `U2` — the two immutability rules may be unreachable through this API.**
  `PatchUserRequest` declares only `givenName`, `familyName` and `status`, so a PATCH carries
  no `email` and no `tenantID` for `UserEmailIsImmutableNotification` /
  `UserTenantIsImmutableNotification` to fire on. The suite attempts both anyway — a body
  carrying an extra key — and **whatever the service answers, the case asserts the DTO gate**:
  a field the write DTO does not declare must not reach the aggregate. If the framework
  rejects the unknown key, that is the assertion; if it silently drops it, the immutability
  notifications are unreachable and this plan records them as such rather than claiming
  coverage. **This is the one row whose expected status the plan states as a DISJUNCTION,
  and it is stated here, before the first request, precisely so it cannot be filled in from
  an answer.**
- **`?search=`, exports and gRPC** — none is wired; §1's `M8.3` proves the refusal, not the
  capability.
- **`EmailVerifiedAt` being SET** — no flow in this service ever writes it. `M3` asserts it
  is `null` at birth; nothing proves a transition that has no producer.

---

## 2. Data hygiene

**Inherited verbatim from `tenant-contract` §2 and unchanged.** The suite runs against the
throwaway database `authcore_qa`, selected by `qa/microservice.qa.yaml` through
`OMNICORE_CONFIG_PATH`; `qa/run.sh` DROPs and CREATEs it before every run, and migration 0012
re-seeds the catalog. No Mongo, no CDC: dropping the database IS the whole reset, and there is
no projection to clear and no relay to drain.

**Residue:** none that survives a run — the database is recreated. Within a run, this lane
leaves archived users, archived collection entries and the tenants/groups/roles/claims it
created; every e-mail and every handle is namespaced by `$QA_RUN_ID`.

**Two new principals, built the same way the other six are — through the service's own
documented flow, nothing invented:**

- **Principal I — `qa-userop`**, in `$QA_TENANT_SCOPED`: holds `user:insert`, `user:update`,
  `user:archive`, `user:read`, `user:grant`, `user:set-claim`, plus the reads a user operator
  needs (`group:read`, `group:insert`, `role:read`, `role:insert`, `claim:read`,
  `claim:insert`, `tenant:read`) — and **deliberately NOT `permission:archive`**, which is
  exactly the permission `U13`'s and `U14`'s escalation negatives need it to lack. The role
  and the group that CONFER `permission:archive` are created by the ADMIN, because Role's own
  escalation rule would refuse I that creation; the question these rows ask is whether I may
  **attach** them.
- **Principal J — `qa-usernogrant`**, same tenant: holds `user:read` and `user:update` and
  **not** `user:grant`, **not** `user:set-claim`, **not** `user:reset-password`. It is the
  only caller for which the four-way verb split is visible — it may rename a user and must be
  refused on all six collection routes and on the reset.

A failure to build either is **not fatal**: the affected rows skip loudly and the report
prints them in the SKIP column.

**One fixture note that is a decision, not a detail.** Every principal this suite builds is
born `must_change_password = true` and its first token therefore carries one permission
(`U23`). `run.sh`'s `make_principal` already rotates for that reason. The `U23` rows must
capture the PRE-rotation token deliberately, so `make_principal` gains an optional flag that
echoes it alongside the rotated one — a change to the runner, listed in §5.

---

## 3. Security — what this round ADDS

`qa/security.sh`, family **S7**. §3a (the 401 family), §3b (the public-route split) and the
framework's appended surfaces are **inherited** from the four approved rounds and not
repeated. Every valid token below comes from **the service's own login route** — this service
is its own IdP (`auth.issuer.enabled: true`), so nothing is invented and no key is forged.

### S7.1 — Layer 1: the permission gate, per route and per surface

Fourteen REST routes and fourteen GraphQL fields, gated by **seven** distinct literals —
`user:read`, `user:insert`, `user:update`, `user:archive`, `user:grant`, `user:set-claim`,
`user:change-password`, `user:reset-password`. Per route, **both** directions:

- **the negative** — principal **B** (`tenant:read` only) → **403** with the
  missing-permission key, on all fourteen;
- **the complement** — principal **I** → 2xx on the twelve it holds, because a gate that
  refuses everyone is also broken;
- **the split** — principal **J** is refused on the six collection routes and on the reset
  while succeeding on the PATCH, which is the only way the four-verb split is visible at all.
  A and B answer identically on all fourteen either way.

**Per surface, not by analogy.** A route gated on REST is not thereby gated on GraphQL, and
"the other surface forgot the gate" is a real regression that only a per-surface row catches
— which matters more here than anywhere, because §0c item (a) shows the specs believed these
fields were not on GraphQL at all.

### S7.2 — Layer 2: identity-derived row rules

The two credential routes are **the only `BuildRules` clauses in this service that read the
principal's SUBJECT** rather than its tenant. `U19` proves the refusals as business rules;
`S7.2` proves them as a boundary, with two calls that differ only in **who is asking**:
principal I changing I's own password (204) vs I changing J's (403), and I resetting J's
(204) vs I resetting I's own (403).

`S7.2c` — **a holder of `user:reset-password` cannot launder a stolen token.** The composite
case the two halves exist for: with `user:reset-password` and nothing else, every route to
replacing one's OWN credential without proving the previous one is closed.

### S7.3 — Layer 3: tenant scoping, and the leak that is a 200

- a token with no tenant claim, where `tenant.required: true` → **403** (inherited);
- **cross-tenant READ** — principal I lists users and the foreign tenant's rows are simply
  **not there**; the by-id read of a foreign user → **404**, never 403 and never the row. An
  isolation leak here is a `200`, which is exactly why it needs its own case rather than a
  status assertion;
- **cross-tenant WRITE** — creating a user with another tenant's `tenantID`, and archiving a
  foreign user → **403 `TenantMismatchNotification`**;
- **the `*:*` bypass acts INSIDE another tenant, never ACROSS two** — the admin creates a
  user in the scoped tenant and may then attach only that tenant's groups, roles and claims;
  a mix of two tenants' rows in one request is refused by `U8`/`U9`/`U10`.

### S7.4 — `ReadCriteria.Restrict`: none, and that is asserted as a posture

`spec.md` §9 records the decision: no field-level read authz on this entity, because the
hash is off the wire for every caller by construction, which is stronger than restricting it.
So there is **no `FieldAccessForbiddenNotification` to assert here** — and the plan says so
rather than leaving the reader to wonder. What replaces it is `U25`/`M8.14–17`: the field is
unreachable for everyone, including a `*:*` operator, which `S7.4` asserts by running the
four oracle cases **as the admin** and getting the same four 400s principal B gets.

### S7.5 — What stays UNPROVEN, printed as SKIP and not only written here

- **The `user:change-password` deployment story.** §10 records that the permission must reach
  every user or nobody can rotate their own credential. The suite proves the RESTRICTED
  token always carries it (`U23`, embedded), which is what keeps the flow from deadlocking —
  but it does not prove any tenant's default role actually grants it, because no such
  default role exists in this service to inspect.
- **The `externalValidator` path** — not configured in any profile; nothing to assert.

---

## 4. Out of scope, named plainly

Load and performance · UI · gRPC (not wired) · exports (not wired) · integration events (the
posture has no broker, so publishing is unavailable — `user.created` and `user.archived` are
recorded in §9 as the two facts a future consumer will want, and there is nothing to assert
today) · the `Client` and `Claim` aggregates' own contracts · the token route's own contract
beyond what `U23`/`U24` need.

**The audit lane IS extended** (`qa/audit.sh`, continuing from `A53`): the insert, the patch,
the archive and both credential operations each write one `audit_events` row with the right
`verb`, `kind`, `actor`, `tenant_id` and `actorClaims` — and, the row this round exists for,
**`payload` carrying `***` where the hash would be, with the real hash string appearing
nowhere in the record** (`U25`'s fourth face).

---

## 5. Runner contract, and the files this round touches

**There is still exactly ONE runner.** `qa/run.sh` gains two lane names and two principals;
nothing else about it changes.

**CREATED:**

| file | what |
|---|---|
| `qa/user.sh` | family **M** — the REST contract, ~14 route families |
| `qa/user_graphql.sh` | family **N** — the GraphQL twins |

**EDITED:**

| file | change |
|---|---|
| `qa/run.sh` | `LANES=(… user user_graphql domain security audit)` — inserted after `group_graphql`, keeping the entity order; principals **I** and **J**; and `make_principal` gaining the optional pre-rotation-token echo `U23` needs |
| `qa/domain.sh` | family **U** — the §1b rows above |
| `qa/security.sh` | family **S7** — the §3 rows above |
| `qa/audit.sh` | the §4 extension, from `A54` |
| `qa/lib/common.sh` | `user_email`, `user_body`, `new_user`, `attach_group`/`attach_role_to_user`/`attach_claim`, `new_claim`, and `login_as` — the fixtures family **M** and family **U** both speak |

**EDITED, prose (§0c, maintainer-approved 2026-09-07):**

| file | change |
|---|---|
| `specs/scaffold-entity/user/spec.md` §9 | the GraphQL reach: root-verbs-only → **full parity, 14 fields**, with a dated supersession note |
| `specs/scaffold-entity/user/spec.md` §9 | the computed field: `name` → **`fullName`**, and the two control examples with it |
| `specs/scaffold-entity/user/spec.md` §9 | the mechanism keeping the hash off the wire: `hidden` → **the Response DTOs declare no such field**, with `RedactedField` named for what it actually governs (the framework's own copies). The section's conclusion and its filter-policy warning are unchanged |
| `specs/scaffold-entity/user/spec.md` §10 | the closing note: "neither profile configures `auth.authorization`" → **both do**, `enabled: true` with `tenant.required: true`; the paragraph's warning about the dev-bench stand-down is unchanged and still correct |

Everything else in the runner contract — root resolution from the script's own location,
fail-fast with `--all` for the sweep, per-run namespaced temp files and ports, SIGTERM and a
drain wait, a non-zero exit per lane — is inherited from `tenant-contract` §5 unchanged.

---

## 6. Report contract

Inherited from `tenant-contract` §6 unchanged: `qa/qa-report.md`, rewritten in full after
EVERY lane, `❌ RUN ABORTED` trapped on `EXIT INT TERM`, a row per declared lane with `—` for
one that never ran, SKIP in its own column, and the footer quoted verbatim at hand-off. The
matrix grows by two rows (`user`, `user_graphql`).

---

## 7. The deliberate-RED meta-case

Mandatory, per the skill's final gate: one generated case has its expectation broken by hand,
the suite is run, the case FAILS, and it is restored. The case chosen is **`M8.17`
(`?fields=passwordHash` → 400)** flipped to expect 200 — so the same run also proves the
report renders a RED lane, names the case, prints the real response body and stamps a RED
footer.
