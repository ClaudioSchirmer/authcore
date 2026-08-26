# tasks.md — User

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-25).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the table at the bottom.

- **Pin:** omnicore `v0.60.0` · `omnicore-gen` `0.40.0` · **Dialect:** postgres (only) ·
  **Posture:** Postgres SoR, no Mongo, no broker → relational-served views ·
  **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** `feature/user-aggregate`, cut from `main` after the `Group` merge landed.
  Not stacked on the group branch — `User` needs `groups` and `roles` to exist, and after
  the merge they do.
- **Generation:** `omnicore-gen` — chosen at gate 1d, built at 0.40.0 and regenerated through
  0.42.0 as five reported gaps were fixed upstream: the body-fed field (`source: body`), the
  bypass-yielding owner (`bypassMaySet`), the value-object unwrap in `comparison`, the
  collection-projector name collision, and `nullable` beside `assignedFrom`.
  [`spec.md`](spec.md) §D records what remains hand-written.

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0a | children delta — read WITH domain, application, web, infra, migrations | [`task_children.md`](task_children.md) | **done** |
| 0b | **credential delta** — read WITH the same five. The part of this entity that has no counterpart anywhere else in the service | [`task_credential.md`](task_credential.md) | **done** |
| 1 | domain | [`task_domain.md`](task_domain.md) | **done** |
| 2 | application | [`task_application.md`](task_application.md) | **done** |
| 3 | web | [`task_web.md`](task_web.md) | **done** |
| 4 | infra | [`task_infra.md`](task_infra.md) | **done** |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | **done** |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | **done** |
| 7 | tests | [`task_tests.md`](task_tests.md) | **done** |
| 8 | docs | [`task_docs.md`](task_docs.md) | **done** |

Layers 0a and 0b are not steps of their own — they are the deltas every layer they touch
reads before it runs. They are listed first because the model's two worst traps live in
them, one per delta.

## The six things about this entity that are NOT `Group`

Carried here from the spec so no layer has to rediscover them:

1. **It holds a secret.** Four separate mechanisms keep it out of four separate copies of
   the row, and a fifth (never declaring a filter) keeps it out of the query vocabulary.
   Getting any one wrong is a credential leak rather than a bug. `task_credential.md`.
2. **Two routes carry credentials, and NEITHER is public** *(amended 2026-08-26)*. This
   item described a public, e-mail-identified change-password that was removed the same
   day. What ships is `PATCH /users/{id}/password` (the caller's own, proving the current
   password) and `PATCH /users/{id}/password-reset` (somebody else's, proving nothing),
   both authenticated and each carrying one permission. There is no unauthenticated write
   in the service. `task_credential.md` and `task_web.md`.
3. **Two collections, not one** — and therefore four child operations, four places to
   mis-wire the root-archive handler instead of two, and the escalation check runs at two
   different depths: two hops for a role, **three** for a group.
4. **The owner is CONDITIONAL** (U1b). A superadmin names the tenant in the body; a tenant
   caller inherits it from the claim; a divergent body value is refused rather than
   overridden. This is the one place the entity deliberately diverges from `Role` and
   `Group`, and it fixes a trade-off both of their specs recorded as a loss.
5. **The name is a COMPOSITE value object** spanning two columns, with the full-name method
   inside it and the rendered name served as a computed read field. Same shape the
   permission key already ships — with its own family of boot panics.
6. **Something writes on a REJECTED request** — the lockout counter. Every other write in
   this service happens because the domain accepted the operation, which is why this one is
   NOT a rule: a rejected aggregate write persists nothing, so it lives in the custom
   handler's failure branch instead. `task_credential.md`.

## Framework contract each layer must confirm before it writes

Never from memory — the routed section at the pin is the authority:

- ids are `domain.ID` on the domain and schema side, `string` on the wire, converted at the
  mappers (`table-schema`).
- a domain field carries `labelKey` and nothing else — no `json:`, no `db:`.
- `Modes()` listing Archive ⟺ the schema declares its archive column ⟺ the migration
  carries it. Three places, one fact.
- a `?fields=` opt-in forces every Response field AND every nested response field to
  `*T`/slice with `,omitempty`, and the query Result pointer/slice throughout.
- the ordering vocabulary lives on the Request per field, paired with the `orderBy` switch;
  either half alone fails the boot.
- `HasPermission` panics on any argument containing `*`. `IsSuperAdmin()` is the sanctioned
  `*:*` question.
- a composite value object never reaches the plain field mapping and never declares the
  single-value accessor; both are boot panics that name their own fix (`table-schema`,
  `value-objects`).
- keeping a value out of a RESPONSE and keeping it out of the framework's own COPIES of the
  row are two different declarations, and the second one has two mandatory axes
  (`table-schema` → the redaction section, `audit`).

## Deviations from `spec.md`, and why

*(filled during execution — one row per deviation, with the reason. An empty table at the
end of a build means the tree matches the model exactly.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
| 1 | §2: `EmailVerifiedAt` and `PasswordChangedAt` are nullable, NULL meaning "not yet" | `EmailVerifiedAt` **is** nullable; `PasswordChangedAt` is NOT | `check` refuses `nullable` beside `assignedFrom`, so both were forced non-nullable at first — and `EmailVerifiedAt`, which nothing writes, then rendered the zero instant as `0000-12-31T18:42:28-05:17` in every response. **Reported and fixed upstream at `omnicore-gen` 0.42.0**; the field is nullable again and reads `null`. `PasswordChangedAt` stays non-nullable and that is correct rather than a leftover: a password is required at creation, so it is always written |
| 2 | §9: the computed read field is `name` | `fullName` | the composite field already owns `Name` on this entity, so `name` would have been two things. `?fields=fullName`, and both halves stay filterable and sortable in their own right |
| 3 | §7 U1: a rule freezing `TenantID` on update | no rule | `assignedFrom: identity-claim` + `bypassMaySet: true` puts the field in the INSERT body only — it is absent from the patch request entirely, so there is nothing an update could change it with. The rule would have been dead code |
| 4 | §7 U16 / §10: the read scope as a numbered rule | generated by `authz.dataAccess: tenant`, not declared | it is a filter in `ToCriteria`, not a `BuildRules` rule — which is what §7's U16 note itself says. Recorded so a reader auditing `BuildRules` does not conclude it is missing |
| 5 | §B Q2 / §E: a PUBLIC change-password endpoint, by e-mail, verifying the current password | **REMOVED 2026-08-26, with everything exclusive to it** | it rested on a premise this service does not have — that somebody needing a new password might be unable to obtain a token. There is no login route here, so `mustChangePassword` blocks no authentication and no credential expires; a caller who knows their password authenticates. It therefore did what the reset does, WITHOUT a token. What it was reaching for is a forgot-password flow (e-mail, expiring link), which is real work nobody has started. **Removed with it:** the command, handler, request DTO, route, the `publicRoutes` entry in both profiles, `PasswordHasher.DummyMatches` and its constant-time fixture, `UserRepository.FindOneByEmail`, `InvalidCredentialsNotification` and its seven catalogs, the `failedLoginAttempts`/`lockedUntil` fields, columns and comments, `RegisterFailedCredentialAttempt`, `IsLockedAt`, the two lockout constants, `ActionChangePassword`, `IsCredentialAction`, and every test that only covered them |
| 6 | §B Q2 / §10: the reset accepts **self**, `*:*`, or `user:reset-password` | ~~`user:reset-password` alone, **not** self~~ → **the intent is back, carried by TWO routes** *(amended 2026-08-26, later the same day)* | the original refusal was mechanical: `RequirePermission` expresses exactly ONE permission and the framework refuses to boot a non-public route declaring none (`bootstrap/route_scan.go`, proven by booting with the flag on), so three alternatives were not declarable **on one route**. Splitting the operation removes the constraint instead of working around it — `PATCH /users/{id}/password` (`user:change-password`, self, proves the current password) and `PATCH /users/{id}/password-reset` (`user:reset-password`, somebody else, proves nothing). Each door carries one permission, and `user:reset-password` keeps the exact meaning it already had |
| 7 | — | both credential handlers feed the aggregate's row rules | not in the spec at all, and it was missing: `refuseForeignTenant` asks whether an identity was PRESENT before comparing, so a hand-written command that skips those lines leaves it standing down. A holder of `user:reset-password` in one tenant could have reset a password in another. The same feed now also carries `RequestingUserID`, which is what the two row decisions of deviation 9 compare. Found while applying deviation 6; covered by tests |
| 8 | — | ~~there is **no self-service password change**~~ → **there is one** *(amended 2026-08-26, later the same day)* | `PATCH /users/{id}/password` is the authenticated change carrying the current password that this row asked for, so the gap it recorded is closed. A FORGOT-password flow — e-mail, one-time link, expiring token — is still not started, and that is now the only credential gap this entity ships with |
| 9 | — | each credential route makes its own **row decision**, and they are mirror images | the change refuses any row but the caller's; the reset refuses the caller's OWN. The second half is not symmetry for its own sake: without it a holder of `user:reset-password` points the reset at their own id and replaces their credential without proving the previous one — defeating the change endpoint's `currentPassword` by choosing the other URL. Same id is always the change, a different id is always the reset |
| 10 | — | `mustChangePassword` is **cleared** by the change and **set** by the reset | the flag had no writer that cleared it once the public route went (deviation 5), so every user carried it forever. The change is the operation that legitimately clears it: the caller picked the password and proved the one before it. The reset sets it, because somebody else picked |
| 11 | — | two spec fields exist only for the hand-written operations: `RequestingUserID` (`source: subject`) and `CurrentPassword` (`source: manual`) | both were generator gaps reported upstream and shipped in omnicore-gen 0.43.0. `source: subject` reads `Identity.Subject` rather than `Claims["sub"]`, which matters because the framework builds identities that set Subject and carry no such claim (`web/grpc/posture.go`, `infra/integration/registry.go`). `source: manual` emits the field and nothing else — no DTO, command or OpenAPI schema — which is what keeps `currentPassword` out of the body of the ordinary PATCH |
| — | *(two temporary deviations are GONE)* | — | the `plural: DirectRoles` rename and the `PasswordConfirmation` value object were workarounds for generator defects, reported upstream and fixed in `omnicore-gen 0.41.0`. Both reverted; the route is `/users/:id/roles` as approved, and the confirmation is a plain `string` again |
