# task: web — Permission

## Read first (mandatory, at execution time)

- `openapi` — the route registration channel, the request/response DTO rules, and the
  required-field rule that makes a value-typed scalar query parameter mandatory in the spec.
- `auto-query-handlers` — the response's projection contract: the pointer + omitempty rule
  when `?fields=` is opted into, and the computed tag's placement beside the JSON tag.
- `graphql` — the field registration unit and how a mutation reuses the REST handler.
- `authz-seams` — Layer 1 gating at the registration unit, on both surfaces.
- `reference` — the response's mapping helper.
- Convention: `conventions/web.md`. Layout and naming: `service-layout`.

## Model decisions that touch this layer

From `spec.md` §9, §10:

- **Operations exposed on REST**, all under the collection path:
  - create — 201, full body.
  - partial update by id — 200.
  - archive by id — 204, no body, a `PATCH` on an archive sub-path. **There is no unarchive
    route.** A soft removal must never be wired behind `DELETE`, and no hard-delete
    operation exists here at all.
  - list — paged, with the filters, ordering, projection, archived-inclusion and total-only
    controls named in `spec.md` §9. Free-text search is deliberately **not** declared: the
    backing cannot serve it, and an undeclared control is answered with a typed 400.
  - read by id.
- **The same five operations on GraphQL**, each reusing the identical handler and the
  identical permission, so the two surfaces cannot drift.
- **The wire shape is lean and identical on every operation**: the id, the description, and
  the computed permission string. The resource and the action are **not** response fields —
  they are read into the result to feed the derivation and stop there (`spec.md` §9).
- **The computed response field** is declared beside its JSON tag, naming the two stored
  sources. It is never given a filter tag — that is a boot panic — and it is never offered
  as an ordering token.
- **Filtering is unaffected by that shape.** The three stored fields are all filterable, and
  filters are declared by the request's own tags, which never consult the response. The
  ordering allowlist, by contrast, IS the response's projection schema — so the ordering
  tokens this endpoint accepts are the description and the id, and nothing else. Do not
  declare resource or action as ordering tokens: they would be rejected at runtime anyway,
  and advertising them in the documentation would be a lie.
- **Every response field is a pointer or a slice with omitempty**, because the listing opts
  into field projection. This applies to nested types too, if any appear.
- **Requests never declare the id path segment themselves** — the by-id wrappers own it.
- **Scalar query-parameter fields are pointers**, so no filter renders as required in the
  generated spec.
- **Authorization** per the `spec.md` §10 table, attached at each surface's registration
  unit. The archive operation carries the archive permission; there is no second operation
  sharing it.
- **OpenAPI documentation text** — a summary and a description per operation, in English,
  in the register the tenant routes already use. The create and update descriptions should
  say plainly that the resource and the action cannot be changed after creation, since that
  is the surprising part of this resource.

## Acceptance

- Five REST operations and five GraphQL fields, no more and no fewer.
- No unarchive anywhere; no `DELETE` anywhere.
- The computed field carries a computed tag and no filter tag.
- Every response field is a pointer or slice with omitempty.
- Builds and vets clean.
