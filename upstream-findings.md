# Upstream findings — omnicore-gen

Found while generating the `Tenant` entity of `authcore` on 2026-08-19.

- generator: the build shipped in omnicore plugin `0.21.0` (targets framework `v0.53.0.x`)
- framework pin of the project: `v0.54.0`
- spec that produced them: `omnicore-gen/tenant.omnicore.yaml`
- dialect: postgres · surfaces: REST + GraphQL · read backing: relational

**No framework bug was found.** The one this run did hit — a mutation performed in an
`IfArchive` closure reaching the audit event and the outbox payload but never the row —
was already fixed in `v0.54.0`, whose changelog names that exact rule shape as the
motivation. The project was upgraded mid-run to get it.

---

## 1. BUG — field labels are seeded from the field's `description:`

### What happens

The label catalogs are seeded with each field's whole `description:` text instead of a
short label, in the catalog matching the spec's declared `language:`. The other six
catalogs fall back to the Go field name, which is correct.

So the one language that gets special treatment is the one that comes out wrong.

### Evidence

Spec (`tenant.omnicore.yaml`), `language: en-US`:

```yaml
  - name: Workspace
    labelKey: TenantWorkspaceField
    description: >-
      Immutable handle of the tenant; reaches URLs, logs and external configuration,
      and is the input the public tenant_id is derived from. Never reused, not even
      after archiving.
```

Generated `internal/application/translations/eng.go:39`:

```go
"TenantWorkspaceField": "Immutable handle of the tenant; reaches URLs, logs and external configuration, and is the input the public tenant_id is derived from. Never reused, not even after archiving",
```

Generated `internal/application/translations/ptbr.go:39` — the correct shape:

```go
"TenantWorkspaceField": "Workspace",
```

Live consequence, from a real `POST /tenants/` with a reserved handle:

```json
{"notificationKey":"ReservedTenantWorkspaceNotification",
 "field":"workspace",
 "fieldLabel":"Immutable handle of the tenant; reaches URLs, logs and external configuration, and is the input the public tenant_id is derived from. Never reused, not even after archiving"}
```

### Why it is a bug rather than a preference

A description and a label are different things with different audiences and different
lengths, and the generator already treats them as different everywhere else:

- the **description** is one line explaining what the field means. It correctly becomes
  the database `COMMENT ON COLUMN`, which is what a DBA and a BI tool read.
- the **label** is the field's short human name. It is what a validation payload puts in
  `fieldLabel` and what a CSV/XLSX export puts in a column header.

Seeding the second from the first makes every validation error carry a paragraph, and
will put a paragraph in a spreadsheet header the day exports are enabled.

### There IS a workaround, and the generator handles it well

An earlier draft of this report claimed hand-editing the catalogs would be overwritten on
the next run. **That was wrong, and it was corrected by experiment**: the label was edited
by hand, `generate` was re-run, and the edit survived. The generator detected it by hash,
refused to touch it, and said so in the report:

```
### Yours in a shared file, and out of step with the spec
- TenantWorkspaceField — internal/application/translations/eng.go
…A translation on this list is cosmetic by comparison: the end user simply reads the
older wording. If your version is the better one, put it in the spec; the two will then
agree and it drops off this list.
```

That behavior is exactly right and is not part of the complaint. The applied fix in this
project was to hand-write the five English labels as short labels (`Name`, `Workspace`,
`Tenant ID`, `Description`, `Status`).

### What remains a defect

Two things, and the workaround does not close either:

1. **The seeding rule is wrong.** A description is not a label. Every project that declares
   `language:` gets paragraph-length labels in that one language and correct fallbacks in
   the other six, and only notices when a 422 payload or a CSV header is read.
2. **The advice the report gives cannot be followed.** It says "if your version is the
   better one, put it in the spec" — but there is no key that holds a field's label text.
   `notifications[].text` and `valueObjects[].members[].text` both take per-language text;
   `fields[].labelKey` names the key only. So the entry stays on the out-of-step list
   forever, which trains a reader to ignore that list.

### Suggested shapes

Either stop seeding the label from the description — falling back to the field name in
every language, which is already what six of the seven do and is already right — or add
`fields[].text` with the same seven-language shape `notifications[].text` already has. The
second also makes the report's own advice followable.

---

## 2. GAP — no way to declare a persisted field the SERVER computes from another field

### What is missing

`assignedFrom: identity-subject | identity-claim` covers "the server fills this, so the
client never sends it" — but only for values read from the caller's identity. A field
derived from ANOTHER FIELD of the same entity has no equivalent, so it is generated as an
ordinary field and lands in every write DTO.

### The case that hit it

`Tenant.TenantID` is the tenant's public key: `UUIDv5(fixed-namespace, workspace)`. It is
persisted, indexed, filterable and returned by every read — and it is never proposed by a
caller. The approved model states it is read-only on every surface.

The derivation itself has a clean home: `rules.manual` scoped to `insert`, which assigns
it, and the assignment is idempotent because it is a pure function of an immutable field.
That part works well. What has no home is the *exclusion from the write DTOs*.

### Evidence

Generated `internal/web/requests/insert_tenant.go`:

```go
type InsertTenantRequest struct {
	fwrequests.Auto
	TenantID    domain.ID `json:"tenantID" example:"a3f1c07e-…"`
	Name        string    `json:"name" …`
	…
}
```

Live, against the running service:

```
POST /tenants/  {"tenantID":"00000000-0000-0000-0000-000000000000","workspace":"acme-comercio",…}
201  {"tenantID":"427a1df1-e2a0-53a9-8f53-fe46edad93f1", …}
```

The caller's value is **accepted and silently ignored**. `PATCH` is louder — the
`immutable` rule answers 422 — but the field is still in the request type and in the
published OpenAPI, documented as writable while doing nothing.

### Why `assignedFrom` cannot be stretched to cover it

Declaring `assignedFrom: identity-subject` would remove the field from the write DTOs and
have the generated mapper fill it from the token, and a manual rule could then overwrite
it. That produces the right bytes and a spec that lies about where the value comes from,
so it was not done.

### Suggested shape

A third `assignedFrom` value — `derived`, or `rule` — meaning "the server assigns this;
keep it out of every write request and command, and out of the OpenAPI request schema".
The generator would not need to know HOW it is computed: the existing `rules.manual` entry
already covers that, and the report already lists it as something the implementer owes.

---

## 3. Observation, not a bug — a generated fixture can be internally inconsistent

`validTenant()` in the generated `internal/domain/tenant_test.go` pairs a literal
`TenantID` with a `Workspace` that does not derive it. Harmless today: the tests that use
it go through the insert path, where the derivation rule overwrites the field before
anything compares them.

It becomes a false failure the day an emitter adds an update-path "a valid entity is
accepted" case, because on update the derivation does not run and the consistency rule
fires — and the fixture, not the rule, would be what is wrong.

Nothing to fix while the generator cannot know that one field derives another (finding 2).
Worth knowing if finding 2 is ever addressed: the fixture builder would then have the
information to keep the pair consistent.
