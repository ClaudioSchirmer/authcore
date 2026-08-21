# task: web — Role

Model: `spec.md` §3, §8, §9, §10. Convention: `conventions/web.md` +
`conventions/aggregate-children.md`. Layout/naming: `service-layout.html`.

## READ before writing (mandatory)

- `openapi.html` — the mount channel, the required-field rule for query parameters, the
  documented 403 emission.
- `auto-handlers.html` — which handler each operation gets, strict vs lenient by the
  operation's field contract.
- `auto-query-handlers.html` — the projection opt-in and what it demands of a response type.
- `graphql.html` — the registration units and how a mutation with a path id is expressed.
- `authz-seams.html` Layer 1 — the permission option on both surfaces, and the boot scan
  that refuses a non-public route without one.
- `custom-query-handler.html` — only if a read needs a shape the auto handler will not serve.

## What to build

**Operations, per §10's table** — five on the root (create, patch, archive, list, by id) and
two on the child (grant, revoke). Every one of them declares its permission on BOTH surfaces;
the child pair rides the root's update verb per §10.

**Route shapes.** The child operations take an extra path segment for the child id. That
segment is **never** named as the reserved id path token — the by-id specification owns that
one, and declaring it is a boot panic.

**The revoke operation is `PATCH …/archive`, never `DELETE`.** The row lingers with an
archive stamp; a delete verb over a soft removal is a lying contract. This is the same rule
the root already follows.

**⚠️ Trap 1, the one that costs a silent data loss:** the root-archive auto handler is
instantiated **at most once per surface**. If the revoke route reaches for it, the request
type-checks, boots and answers 200 while archiving the entire role. The revoke operation
mounts the child command written in the application layer, and follows that operation's own
field contract like any other.

**Request and response types.** Every scalar query parameter is a pointer or a slice — a
value-typed scalar renders as REQUIRED in the generated specification and Swagger then
refuses the call without it. Because the listing opts into field projection, **every** field
of the listing response **and of its nested child response type** is a pointer or slice with
the omit-empty tag; a bare value type or a tag missing omit-empty is a boot panic, and the
nested type is the half that gets forgotten.

**The read payload carries the catalog reference and no rendered permission string** (§2,
Q2). No computed field is declared here; there is nothing to derive from.

**Filters and sorts** exactly as §9 tabulates them. `?search=` is NOT declared — the
relational backing answers it with a typed 400, and declaring it would advertise a capability
the server refuses. The same applies to any filter or sort over a child field: it is
unavailable on this posture, so it is not offered.

**The documented text must not promise an unarchive** — §5 has none. The archive operation's
summary says the row stays as history, mirroring what the catalog entity already does.

## Acceptance

- Every non-public operation declares a permission on both surfaces.
- The root-archive handler appears at most once per surface; the revoke route does not
  reference it.
- No reserved id path token declared on any by-id request.
- Every scalar query field is a pointer; every projected response field, nested included, is
  a pointer or slice with omit-empty.
- Builds and vets clean.
