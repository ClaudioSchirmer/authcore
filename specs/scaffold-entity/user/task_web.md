# task_web.md — web

Model authority: [`spec.md`](spec.md) §8, §9, §10, §E. Read
[`task_children.md`](task_children.md) and [`task_credential.md`](task_credential.md) first.

## The operations to mount

**Root, five:** create · list · read one · partial update · archive.
**Collections, four:** join a group · leave a group · grant a role · revoke a role. Every
removal uses the archive verb, never the purge verb.
**Credential, two:** the public change, and the reset.

**GraphQL carries the root verbs only.** The four collection operations and the two
credential operations are REST-only — and the public one especially: the authentication
bypass matches an exact method and path, and a single GraphQL endpoint cannot be made
selectively public per field.

## The authorization map

Five permission literals gate the root and the collections; the two credential operations
are gated differently and neither carries the update permission:

- the four root write and read operations take the entity's own verbs;
- all four collection operations take the **grant** verb — one verb for both edges, because
  they confer the same kind of thing and the group edge strictly dominates the role edge;
- the **reset** accepts three acceptors: the caller being the target, the caller crossing the
  row scope, or the caller holding the dedicated reset permission. The first must never
  depend on a grant — gating self-service on a permission lets an operator lock a whole
  tenant out of their own credentials simply by never issuing it;
- the **change** is public and carries no permission at all.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| the request and response shapes, and the by-id binding | `openapi` · `reference` |
| the listing's declared vocabulary — filters, ordering, projection, the reserved controls | `auto-query-handlers` |
| a hand-written route beside the generated ones | `custom-query-handler` · `custom-command-handler` |
| the GraphQL surface and where authorization attaches on it | `graphql` |
| where a permission attaches per surface, and the three access layers | `authz-seams` |
| the bypass that makes a route public, and its exact declaration | `auth-middleware` · `yaml-reference` |
| file layout and naming for this layer | `service-layout` |

Convention: `conventions/web.md`.

## Traps specific to this layer

- **Never declare the id path segment on a by-id request** — the framework owns it, and
  declaring it is a boot panic.
- **The projection opt-in has a cost**: if the listing declares it, every response field and
  every nested response field must be optional and omit-empty, and the query result must be
  pointer-or-slice throughout. Two separate boot guards.
- **Every scalar filter parameter is a pointer or a slice** unless the model declares it
  required. A value type renders it REQUIRED in the generated documentation and the try-it
  page refuses the call without it.
- **The ordering vocabulary is a pair** — the per-field tag and the switch. Either half alone
  fails the boot.
- **The root-archive handler is instantiated once per surface**, and there are four child
  routes to wire it to by mistake.
- **The public route shares a shape with the by-id patch route.** Registration order is
  load-bearing: the public one first, with the reason written on the line, and the by-id
  route parsing its segment as an identifier so an inverted order cannot silently swallow it.
- **The public route must be declared in the bypass list of BOTH profiles.** The production
  one is the one that gets forgotten — and once the declarative authorization layer is
  switched on, a non-public route with no permission fails the boot scan, which is the good
  direction.
- **Nothing about the credential reaches a response.** The change operation answers with no
  body or a minimal one; it must never answer with the user's row, which would turn a
  credential check into a profile read.

## Acceptance check

- Eleven REST operations, five GraphQL ones, no more and no fewer.
- The declared listing vocabulary matches §9's table exactly — and the hash appears in no
  filter and no ordering token, on any surface.
- A control the listing does not declare answers a typed 400 rather than being ignored.
- The generated documentation page renders every operation with its example values filled —
  no parameter rendering as a bare placeholder.
- The public route reaches its own handler; an unauthorized status there means the by-id
  route swallowed it.
- Build and vet clean.
