# task_web.md — web

Model authority: [`spec.md`](spec.md) §8, §9, §10. Read the two deltas first.

## What this layer must contain

**The ten operations of §9's table on REST**, each mounted with the permission that table
names, and the same set on GraphQL with the same permissions at the registration unit.
Authorization is not surface-specific: one handler, one permission, declared at each
surface's own registration point.

**Requests and responses.** No request declares the identifier as a path value on a by-id
shape — the by-id specification owns it, and declaring it is a boot panic. The projection
option is served, so every field of every list response and of every nested response type
must be optional and omit-when-empty; a plain value type there is a second boot panic, and
the two are separate guards.

**Every scalar filter parameter is optional** unless the spec declares that filter required.
A value-typed scalar renders as required in the published document, and one accidental value
type turns an optional parameter mandatory.

**Neither hash nor the retiring deadline appears in any request, response, filter, sort or
projection vocabulary.** This is the one protection nothing automatic provides.

**The two credential-bearing responses render the minted secret**, and no other response in
this entity does.

**Descriptions, summaries and examples are filled** — every example renders as a real value
of the right shape, never as a bare type name.

## What to read before writing — routed sections at the pin

`openapi` (the required-field rule) · `graphql` · `auto-handlers` (strict full-body versus
lenient partial contracts) · `custom-query-handler` · `authz-seams` · `reference`.
Convention: `conventions/web.md`.

## Acceptance

- The published document lists exactly the ten operations, each with its permission and no
  others.
- No by-id request declares the identifier as a path value.
- Every field of every list response and nested type is optional and omit-when-empty.
- The root-archive handler is instantiated once per surface and in no child route.
- A request naming a hash in the projection vocabulary is a typed refusal.
