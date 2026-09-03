# Finding — `unique:` cannot name the field its 409 answers on, and GraphQL carries no descriptions

Filed 2026-09-02 · authcore · omnicore `v0.71.0` · omnicore-gen `0.59.0` (plugin)
Status: OPEN upstream. Worked around in this service by two ADOPTED hand edits.

Two gaps, found while making the permission catalog's 409 name a field a consumer can read.

## 1. `unique:` has no wire-name target

The `Permission` entity stores one concept — `PermissionKey` — across two columns
(`resource_name`, `action_name`). The Go identifier of the composite is `Key`, and `Key`
reaches **no** wire in **any** direction: it is not in a request body (writes carry
`resource` and `action`), not in a response (reads carry the computed `permission`), not a
filter and not a projection path. Yet the duplicate answer named it:

```
POST /permissions {"resource":"tenant","action":"read", …}
409 → {"field": "key", "fieldLabel": "Permission", "semantic": "Conflict"}
```

A consumer highlighting the offending input receives a field name it never sent and can
never read back.

The generator's vocabulary (`omnicore-gen explain keys`) offers, under `fields[].unique`:
`enforce`, `notification`, `scope`, `within`. **None of them names the field the answer is
reported on.** The emitted name comes from the Go identifier, in two independent places:

| file | emission |
|---|---|
| `internal/domain/permission.go` | the service pre-check: `r.AddNotification("Key", …)` — the name is camelCased for the wire AND used as the lookup key for the `labelKey` struct tag (`domain/field_label.go::resolveLabelKey`) |
| `internal/infra/permission_repository.go` | the unique-index backstop: `write.ConstraintBinding{… Field: "key"}` |

Both had to change together: otherwise the race between the pre-check and the commit
decides which name the caller sees.

### What we did here

The framework already has the right seat — `NotificationMessage.Override`, whose documented
purpose is to "rewrite a field's wire name without losing the structured Path or the
original FieldName for diagnostics" (`domain/notification.go`, precedence
`Override > rendered Path > FieldName`). So the pre-check now emits:

```go
r.AddNotificationMessage(domain.NotificationMessage{
	Path:         []domain.PathSegment{{Name: "Key"}},
	Override:     "permissionKey",
	LabelKey:     "PermissionKeyField",
	FieldValue:   e.Key.String(),
	Notification: PermissionAlreadyExistsNotification{},
})
```

and the binding says `Field: "permissionKey"`. Both files are `omnicore-gen adopt`ed, so
regeneration keeps the edit and prints it as adopted.

### Suggested shape upstream

A wire-name key on `unique:` that the generator threads into BOTH emitters — the
`AddNotification` call site (as an `Override`) and the `ConstraintBinding.Field` — so they
cannot drift apart:

```yaml
unique:
  enforce: service-precheck+constraint
  notification: PermissionAlreadyExistsNotification
  scope: active-only
  field: permissionKey        # the name the ANSWER uses; defaults to the Go identifier
```

It matters most exactly where it is missing: a COMPOSITE with `hidden: true` parts is the
one shape whose Go identifier is guaranteed to reach no wire.

## 2. The GraphQL schema carries no field descriptions

The same explanation — `permissionKey` = `resource:action`, written in as two fields, read
back as one — was documented in the four OpenAPI route descriptions
(`fwopenapi.Doc{Description: …}`). There is **no equivalent seat on the GraphQL side**:
in `omnicore@v0.71.0`, `web/graphql/introspection.go` is the ONLY file in `web/graphql`
that mentions `Description`, and it merely reads `def.Description` / `f.Description` and
nil-elides them. Nothing in the framework or in the generated code ever populates those
fields, so a GraphQL client introspecting the schema gets `description: null` on every type
and every field, while the REST client reading the same service gets prose.

Worth considering upstream: carry the spec's `description` (fields, and `read.computed`)
into the SDL/introspection the way the OpenAPI generator already does. The text exists in
the spec; only the plumbing is missing.

## Environment

- omnicore `v0.71.0`, omnicore-gen `0.59.0`, Postgres 17, relational read models
- `auth.mode: jwt`, reproduced with a `*:*` token — no authorization layer involved
- Verified by `./qa/run.sh --all`: 446 cases, 2 lanes, 0 RED, with `E1c` asserting the new
  field name and `E1d` the echoed `resource:action` value
