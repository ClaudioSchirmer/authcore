# task — web

Model authority: `spec.md` §8, §9, §10. Layout/naming/granularity: `service-layout.html`.

## Read BEFORE generating (mandatory, at pin v0.54.0)

| Section | Why this layer needs it |
|---|---|
| `openapi.html` | route registration, the request/response binding, and the required-field rule that decides whether a query parameter renders mandatory |
| `reference.html` | the request and response wrappers, and the export column source |
| `graphql.html` | the surface this entity brings up for the first time, and how authorization attaches per field |
| `authz-seams.html` | the permission gate, and the read-restriction seam this entity deliberately does not use |
| `auto-handlers.html` | which handler each route mounts and its field contract |
| `status-mapping.html` | so the documented responses match what the domain actually emits |
| `service-layout.html` | where requests, responses and route registration live |

**v0.54.0 notes that change this layer:** the documented responses grow by two. Every root
write — the partial update, the archive and the unarchive — can now answer **409** for a
stale-revision write, and archiving a row that is not there answers **404** rather than
committing an event about nothing. Both are part of the contract and belong in the OpenAPI
responses, not discovered by a caller in production.

## What to build

**REST**, exactly the six operations of `spec.md` §9 and nothing more: the list, the
create, the read-one, the partial update, the archive and its undo. No export operations —
`spec.md` §B Q3 declined them, and an unasked endpoint is the failure this plan exists to
prevent.

**The request DTOs.** The write ones carry only what `spec.md` §9 says they carry: the
public tenant id is a member of none of them, on either verb. The list request opts into
exactly the reserved read controls the spec's table marks served, and declares the filter
vocabulary of its filter table.

**The response DTOs.** Every read response carries the public tenant id — it is the value a
consuming service resolves a token claim by. Because the projection control is served, every
response field and every nested type must be a pointer or a slice with an omit-when-empty
tag; a bare value type there is a boot panic, not a style preference.

**GraphQL**, per `spec.md` §9: queries for the two reads, mutations for the four writes.
This entity is the one that brings the declared-but-unmounted surface up, by implementing
the framework's GraphQL feature contract. Authorization attaches at each registration unit
with the same permission strings as REST — authorization is not surface-specific.

**Authorization**, per `spec.md` §10: the four permission strings, mapped to operations
exactly as the spec's table says. No row-scoping predicate anywhere — `spec.md` §B Q4
settled that any holder of the permission sees and edits every row, and that is a decision
recorded rather than an omission.

## Acceptance

- Six REST operations, no seventh. No export surface.
- The public tenant id appears in every read response and in no write request.
- Every scalar query-tagged field is a pointer or a slice unless the spec declares that
  filter required — a value scalar renders the parameter mandatory in the spec, and one
  accidental value type makes Swagger refuse the call without it.
- No by-id request declares the id path segment itself; the by-id wrapper owns it, and
  declaring it is a boot panic.
- Every response field and nested type is pointer-or-slice with omit-when-empty, because
  the projection control is served.
- 409 and 404 are documented on the writes that can answer them.
- Layout and naming match `service-layout.html`.
- `go build -tags postgres` and `go vet -tags postgres` clean.
