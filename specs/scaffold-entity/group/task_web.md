# Task 3 — web

## Docs to READ (mandatory, at the pin)

- `openapi` — route registration, the documented request/response shapes, and the
  **required-field rule for query parameters**.
- `reference` — the request/response tag vocabulary.
- `auto-handlers` + `auto-query-handlers` — which handler each operation mounts, and the
  reserved read controls.
- `graphql` — the GraphQL surface and where authorization attaches on it.
- `authz-seams` — the three layers, and where the permission gate attaches per surface.

Convention: `conventions/web.md`. Read `task_children.md` too. Layout and naming per
`service-layout.html`.

## Operations to expose

**REST**, on `/groups`:

| Operation | Route | Permission |
|---|---|---|
| list | `GET /groups` | `group:read` |
| create | `POST /groups` | `group:insert` |
| read one | `GET /groups/{id}` | `group:read` |
| partial update (name, description) | `PATCH /groups/{id}` | `group:update` |
| archive — **one-way, there is no undo** | `PATCH /groups/{id}/archive` | `group:archive` |
| attach a role | `POST /groups/{id}/roles` | **`group:grant`** |
| detach a role | `PATCH /groups/{id}/roles/{entryId}/archive` | **`group:grant`** |

**GraphQL** carries the **root verbs only** — the listing, the by-id read, create, patch and
archive. The two collection operations are REST-only, mirroring how `Role` ships.

**`group:grant` is a fifth verb and is intentional** (spec §10). Do not "fix" the two
collection routes to ride `group:update`.

No unarchive route, on the root or the entry. No purge route anywhere. No exports.

## Read surface

- Reserved controls **served**: pagination, `orderBy`, field projection, total-only, and
  include-archived. **Free-text search is NOT served** — a relational-backed view answers it
  with a typed 400, so declaring it would advertise a capability the server refuses.
- The ordering vocabulary is declared **on the request, per field**, paired with the
  `orderBy` switch. Either half alone fails the boot. Sortable: the tenant reference, the
  handle, the display name. **Not** the description.
- Filters per field exactly as spec §9's table.
- **Every scalar filter parameter must be a pointer or a slice.** A value-typed scalar
  renders as REQUIRED in the OpenAPI document, and Swagger then refuses the call without it.
  Spec §9 declares no required filter.
- Field projection is opt-in here, so **every response field and every nested response field
  must be a pointer or a slice with omit-empty**. A bare value type is a boot panic, not a
  lint warning.
- The entry response carries the entry id, the role reference, and the three values the read
  join fills — `roleKey`, `roleName` and `archivedAt` (spec §2). All three are **read-only**:
  they appear on the response and on no request DTO, no command and no filter vocabulary.
  `archivedAt` is nullable, so it is a pointer with omit-empty like every other field here.

## Acceptance check

- The seven REST operations above, and only those; the five GraphQL root verbs, and only
  those.
- The two collection routes carry `group:grant`; the root routes carry the other four verbs.
- The root-archive auto handler appears at most once per surface, and not on a collection
  route (`task_children.md`).
- No request declares a path segment for the by-id parameter — the by-id shape owns it, and
  declaring it is a boot panic.
- Every scalar filter parameter is a pointer or slice; every response field is a pointer or
  slice with omit-empty.
- `gofmt -l` prints nothing; `go vet` and `go build` clean.
