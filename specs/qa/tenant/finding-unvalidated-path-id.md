# Finding — an unvalidated `:id` path segment surfaces as HTTP 500

**Component:** omnicore (framework) · **Version:** v0.69.0
**Severity:** every by-id route of every entity, read and write, on both surfaces
**Found by:** authcore's contract QA suite (`qa/tenant.sh`, case `I5`), 2026-09-01
**Not reproducible-dependent:** the mechanism is framework code, not service code

---

## Summary

A request whose `:id` path segment is not a UUID reaches the database verbatim and the
driver's type error escapes as a generic **500 Internal Server Error**. Nothing between
the wire and the SQL bind validates the segment.

The framework already models this exact rejection — `domain.ID.IsValid()` raises
`InvalidIDUUIDNotification`, which is translated in all seven catalogs — and no by-id
route calls it. The seam exists and is dead.

## Reproduction

Any omnicore service on a UUID primary key. Below is authcore at v0.69.0, Postgres,
authenticated with a token holding `*:*`:

```
GET /tenants/not-a-uuid
→ 500
{"success":false,"status":500,"description":"Internal Server Error",
 "errors":[{"context":"Server","messages":[
   {"notificationKey":"InternalServerErrorNotification",
    "message":"Internal server error.","semantic":"Internal"}]}]}
```

Server log:

```
level=ERROR msg="pipeline exception"
  error="ERROR: invalid input syntax for type uuid: \"not-a-uuid\" (SQLSTATE 22P02)"
level=WARN  msg="http.inbound" method=GET path=/tenants/not-a-uuid status=500
  route=/tenants/:id
```

## Blast radius — measured, not inferred

| request | answer |
|---|---|
| `GET /tenants/not-a-uuid` | 500 |
| `PATCH /tenants/not-a-uuid` | 500 |
| `PATCH /tenants/not-a-uuid/archive` | 500 |
| `PATCH /tenants/not-a-uuid/unarchive` | 500 |
| `GET /users/not-a-uuid` | 500 — a different aggregate, same mechanism |
| `{ tenant(id: "not-a-uuid") { id } }` | 200, `errors[]` **untyped** (see below) |
| `mutation { archiveTenant(id: "not-a-uuid") { success } }` | 200, `errors[]` untyped |

Six `SQLSTATE 22P02` entries in one probe run across two aggregates.

**The GraphQL half is a second, distinct symptom.** The refusal arrives with no
`notificationKey` at all — only `semantic: "Internal"`:

```json
{"data":{"tenant":null},
 "errors":[{"message":"internal server error","path":["tenant"],
            "extensions":{"semantic":"Internal"}}]}
```

Every other refusal on this surface carries the typed identity
(`extensions.notificationKey`, `field`, `semantic`) — a missing record, a duplicate
handle, a schema violation all do. This one does not, so a GraphQL consumer cannot tell
the failure apart from a genuine server fault.

## Root cause

The three by-id wire wrappers bind the path segment and hand it straight on, with no
validation between:

| file | line | code |
|---|---|---|
| `web/handle_command.go` | 60 | `cmd.SetPathID(c.Params("id"))` |
| `web/handle_command_with_body.go` | 175 | `cmd.SetPathID(c.Params("id"))` |
| `web/handle_query.go` | 296 | `q.SetPathID(c.Params("id"))` |

`domain.NewID(s)` (`domain/id.go:35`) is an opaque wrapper by design — it accepts any
string. Downstream, the relational reader binds that string into `WHERE id = $1` against
a `uuid` column, and Postgres raises `22P02`. The error is not a `NotificationCarrier`,
so `fwweb.ErrorHandler` falls through to `InternalServerErrorNotification` → 500.

`application/handlers/guards.go:16` (`RequirePathID`) is **not** a hook for this: it
guards the empty string only, and it *panics* — which is itself routed to a 500 by
design, since it exists to catch a wiring mistake, not a bad request.

### The framework already has the vocabulary

```go
// domain/id.go
func (id ID) IsValid(fieldName string, ctx *NotificationContext) bool {
    if _, err := uuid.Parse(id.value); err != nil {
        ctx.AddNotificationMessage(NotificationMessage{
            FieldName: fieldName, FieldValue: id.value, Err: err,
            Notification: InvalidIDUUIDNotification{},
        })
        return false
    }
    return true
}
```

`domain/id.go`'s own comment on `UnmarshalJSON` says it plainly: *"Like NewID, it performs
no uuid validation: the ID is an opaque identity wrapper and **IsValid is the explicit
validation seam**."* `InvalidIDUUIDNotification` is declared at
`domain/notification_core.go:92` and translated in all seven catalogs
(`"Invalid primary key."` / `"Chave primária do registro é inválida."` / …).

Nothing in `web/` calls it on a path id.

## Ownership — framework, not the generator, not the service

This was checked as three separate questions rather than assumed.

**Not the service (authcore).** `omnicore-gen doctor` reports no drift on any of the
seven entities: every generated route file is byte-for-byte as the generator wrote it, so
no hand edit is in play. Nothing in the service's domain, application or infra layers
participates in binding a path segment.

**Not the generator (omnicore-gen).** `omnicore-gen explain` — across `keys`,
`vocabulary`, `coverage` and `ownership` — contains no key concerning id validation, uuid
parsing or the path id. There is no spec sentence a service could write to ask for this,
and there is nothing the generator withheld. More to the point, the generated route is
*correct*: it calls the framework's canonical wrappers
(`fwweb.QueryByIDSpec`, `fwweb.CommandByIDSpec`, `fwweb.CommandWithBodyIDSpec`), and
those wrappers do the `c.Params("id")` binding **themselves**. The generated file has no
seat where a check could go. The only way the generator could work around this would be
to stop using the framework's own by-id API and hand-roll a route — which would be the
wrong fix, and would still leave the GraphQL constructors (`QueryByID`, `MutationByID`)
broken, since they bind the id the same way.

**The framework.** The binding, the opaque `ID`, the dead `IsValid` seam, the untyped
GraphQL rendering and the 500 fallthrough are all omnicore code.

## Design axes — named, not decided

The depth of the fix is the framework team's call. Two questions look independent:

1. **Which status is the contract?** `400` reads as consistent with the framework's own
   `SemanticSchema vs SemanticValidation` rule ("Schema = the consumer didn't honor the
   Request shape") and with how a malformed `?after=` cursor and an unresolvable
   `?fields=` path are already answered. `422` is what the existing
   `InvalidIDUUIDNotification` yields untouched, since it declares no `Semantic()` and
   falls back to `SemanticValidation`. `404` is a third reading — "no record has that
   address" — at the cost of hiding a client bug behind a normal-looking answer.
   Note: `WithSemantic(...)` is currently declared only on `RequiredFieldNotification`
   (`domain/notification_core.go:30`), so shelving this one as Schema needs either that
   method on the type or a `Semantic()` override.

2. **Where does the check run?** The wire wrappers are five sites (the three REST ones
   above plus `web/graphql`'s `QueryByID` / `MutationByID`, which is what would also fix
   the untyped GraphQL rendering). A single choke point further down would be one site
   covering both surfaces, at the cost of moving a wire-shape check into the application
   layer.

Neither is prescribed here — the finding is that the refusal is untyped and the seam is
dead, whichever shelf it ends up on.

## Environment

- omnicore `v0.69.0`, omnicore-gen shipped with plugin `0.58.0`
- Postgres 17 (`postgres:17-alpine`), `relational.dialect: postgres`, relational read
  models (no Mongo, no CDC)
- `auth.mode: jwt`, `authorization.enabled: true`; reproduced with a `*:*` token, so no
  authorization layer is involved
- Reproduced by `qa/tenant.sh` case `I5`, which asserts 404 and is left RED on purpose
  until the contract is decided upstream
