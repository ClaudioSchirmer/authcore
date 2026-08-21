# tasks.md — Group

**Model authority: [`spec.md`](spec.md), `Status: APPROVED` (2026-08-21).** Nothing here
re-decides the model. Where this file and the spec disagree, the spec wins; where a task
file's mechanical detail contradicts a routed `/docs` section or a layer convention, the
**doc/convention wins** — apply it and record the deviation in the section at the bottom.

- **Pin:** omnicore `v0.56.1` · **Dialect:** postgres (only) · **Posture:** Postgres SoR,
  no Mongo, no broker → relational-served views · **Surfaces:** REST + OpenAPI + GraphQL
- **Branch:** `feature/group-entity`
- **Generation:** `<pending — gate 1d>`

## Layer order and status

| # | Layer | Task file | Status |
|---|---|---|---|
| 0 | children delta (read WITH the layers it touches: domain, application, web, infra, migrations) | [`task_children.md`](task_children.md) | pending |
| 1 | domain | [`task_domain.md`](task_domain.md) | pending |
| 2 | application | [`task_application.md`](task_application.md) | pending |
| 3 | web | [`task_web.md`](task_web.md) | pending |
| 4 | infra | [`task_infra.md`](task_infra.md) | pending |
| 5 | migrations | [`task_migrations.md`](task_migrations.md) | pending |
| 6 | bootstrap | [`task_bootstrap.md`](task_bootstrap.md) | pending |
| 7 | tests | [`task_tests.md`](task_tests.md) | pending |
| 8 | docs | [`task_docs.md`](task_docs.md) | pending |

Layer 0 is not a step of its own — it is the delta every child-bearing layer reads before
it runs. It is listed first because the model's single worst trap lives in it.

## The four things about this entity that are NOT `Role`

Carried here from the spec so no layer has to rediscover them:

1. **G6 asks a third question `Role` never had — same tenant.** `Role` is tenant-scoped
   (`Permission` was not), so an attached role from another tenant is a cross-tenant leak.
   All three questions answer with **one** notification, deliberately: a distinct
   "belongs to another tenant" message is an existence oracle over a competitor's org chart.
2. **`group:grant` is a fifth verb.** The two collection operations do NOT ride
   `group:update`. This is the one place the taxonomy diverges from `Role`'s, and it is the
   decision (spec §10), not an oversight.
3. **G10a is transitive.** The escalation check resolves a role to its permission keys and
   requires the caller to hold every one of them — not one key, a set.
4. **One-way archive hurts more here.** Unarchiving would re-authorize a whole team at once.

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
  either half alone fails the boot (`v0.55.0`, breaking).
- `HasPermission` panics on any argument containing `*`. `IsSuperAdmin()` is the sanctioned
  `*:*` question.

## Deviations from `spec.md`, and why

*(filled during execution — one row per deviation, with the reason. An empty table at the
end of a build means the tree matches the model exactly.)*

| # | Spec says | Built as | Why |
|---|---|---|---|
