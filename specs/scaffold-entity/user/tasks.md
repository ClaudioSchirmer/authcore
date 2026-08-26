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
- **Generation:** `<pending gate 1d>`. The gap that made this gate awkward — no spelling for
  a body-fed, non-persisted field — was **reported upstream and fixed at `omnicore-gen`
  0.40.0** (`source: body`), together with the secondary one (`bypassMaySet`). [`spec.md`](spec.md)
  §D records both, and what remains hand-written either way.

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0a | children delta — read WITH domain, application, web, infra, migrations | [`task_children.md`](task_children.md) | pending |
| 0b | **credential delta** — read WITH the same five. The part of this entity that has no counterpart anywhere else in the service | [`task_credential.md`](task_credential.md) | pending |
| 1 | domain | [`task_domain.md`](task_domain.md) | pending |
| 2 | application | [`task_application.md`](task_application.md) | pending |
| 3 | web | [`task_web.md`](task_web.md) | pending |
| 4 | infra | [`task_infra.md`](task_infra.md) | pending |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | pending |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | pending |
| 7 | tests | [`task_tests.md`](task_tests.md) | pending |
| 8 | docs | [`task_docs.md`](task_docs.md) | pending |

Layers 0a and 0b are not steps of their own — they are the deltas every layer they touch
reads before it runs. They are listed first because the model's two worst traps live in
them, one per delta.

## The six things about this entity that are NOT `Group`

Carried here from the spec so no layer has to rediscover them:

1. **It holds a secret.** Four separate mechanisms keep it out of four separate copies of
   the row, and a fifth (never declaring a filter) keeps it out of the query vocabulary.
   Getting any one wrong is a credential leak rather than a bug. `task_credential.md`.
2. **One route is PUBLIC.** The change-password operation takes no token, is identified by
   e-mail rather than by id, and answers one generic message for every credential failure.
   It is the only unauthenticated write in the service, and it is a login endpoint in
   everything but name. `task_credential.md` and `task_web.md`.
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
| 1 | §2: `EmailVerifiedAt`, `PasswordChangedAt` and `LockedUntil` are nullable, NULL meaning "not yet" | all three NOT NULL, with the **zero instant** meaning "not yet" | `check` refuses `nullable` on a server-assigned field: *"a server-assigned field is always written, so it is never null"*. The model is unchanged — only how absence is spelled. `PasswordChangedAt` is arguably better for it: with a password required at creation it is always written, which the spec's own "NULL only for a user who never had one" could not happen |
| 2 | §9: the computed read field is `name` | `fullName` | the composite field already owns `Name` on this entity, so `name` would have been two things. `?fields=fullName`, and both halves stay filterable and sortable in their own right |
| 3 | §7 U1: a rule freezing `TenantID` on update | no rule | `assignedFrom: identity-claim` + `bypassMaySet: true` puts the field in the INSERT body only — it is absent from the patch request entirely, so there is nothing an update could change it with. The rule would have been dead code. Verified: `PatchUserRequest` carries no `tenantID` |
| 4 | §7 U16 / §10: the read scope as a numbered rule | generated by `authz.dataAccess: tenant`, not declared | it is a filter in `ToCriteria`, not a `BuildRules` rule — which is exactly what §7's U16 note says. Recorded so a reader scanning `BuildRules` for it does not conclude it is missing |
| 5 | the credential notification answers 401 | `semantic: forbidden` (403) in the spec YAML | the vocabulary has no 401. The type and its seven catalogs exist; the hand-written change-password handler answers **401**, because a credential failure is an authentication failure rather than an authorization one |
| — | *(two temporary deviations are GONE)* | — | the `plural: DirectRoles` rename and the `PasswordConfirmation` value object were workarounds for generator defects, reported upstream and fixed in `omnicore-gen 0.41.0`. Both reverted; the route is `/users/:id/roles` as approved, and the confirmation is a plain `string` again |
