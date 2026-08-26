# task_application.md — application

Model authority: [`spec.md`](spec.md) §2, §7, §8, §9. Read
[`task_children.md`](task_children.md) and [`task_credential.md`](task_credential.md) first.

## What this layer must contain

**Commands and their results** for: create, patch, archive, the four child operations, and
the two credential operations. Naming and granularity per `service-layout`.

**Queries** for the by-id read and the by-params listing, each with its result type and its
criteria mapping.

**Input DTOs** for the two collection entries.

**The action-name map**, so the audit trail can tell two operations apart that share a verb.

**The translation catalogs**, all seven, for every notification and every field label this
entity introduces. Real translations — the seven-catalog rule is about the dev's end users
and is never collapsed to the conversation language.

## The decisions that land here

- **The command mapper is the only layer allowed to read the request context**, and it is
  where the two identity values reach the entity: the caller's tenant and whether the caller
  crosses the row scope. The second one is answered with the framework's sanctioned
  superadmin question, **never** by asking the permission probe about a wildcard (it panics)
  and never by hand-reading the claim set (the claim name is configurable).
- **The conditional owner source** is applied here: a caller who crosses the scope supplies
  the tenant; anyone else inherits it from the claim; a divergent supplied value is left for
  the domain to refuse rather than silently overwritten.
- **The hash is produced here**, through the port — never in the domain, and never inline.
- **The patch mapper touches three fields only.** There is no shape in which a credential
  value reaches it.
- **The listing's criteria mapping injects the caller's tenant**, and skips that injection
  for a caller who crosses the scope. This is the read-side scope rule, and it is the only
  protection in this entity that is not a refusal — nothing is rejected; rows simply are not
  there.
- **The rendered full name is filled once**, on the read path, from the composite's own
  method, so every surface agrees.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| the command shapes and their in-transaction slots | `command-handler` · `auto-handlers` |
| an operation the auto handlers do not cover | `custom-command-handler` · `handler-invariance` |
| the read's result type and the mapping into it | `auto-query-handlers` · `custom-query-handler` |
| a derived read value with no column — its sources, and why it cannot be ordered or filtered | `auto-query-handlers` |
| where the identity is allowed to be read, and the three access layers | `authz-seams` |
| what one write touches end to end | `lifecycle-map` |

Convention: `conventions/application.md`.

## Traps specific to this layer

- **The full-update mapper may run twice** on a shared-identity upsert path. Not applicable
  to this entity's storage kind, but the mappers must stay pure and idempotent anyway.
- **The partial-update mapper must not treat "absent" as "clear"** — that is what separates
  the two update shapes, and this entity ships only the partial one.
- **A derived read value cannot be ordered or filtered.** Both are typed 400s, and the
  Request DTO is what decides whether a token is even declared.
- **Every scalar filter field must be a pointer or a slice** unless the model declares that
  filter required — a value type renders the parameter REQUIRED in the generated API
  documentation, and one accidental value type makes the whole listing uncallable without it.

## Acceptance check

- Every operation in §9's endpoint list has its command or query.
- The action-name map distinguishes the operations that share a verb.
- All seven catalogs carry every new notification and field label, actually translated.
- The read-side scope injection is present, and is skipped for the scope-crossing caller.
- Build and vet clean.
