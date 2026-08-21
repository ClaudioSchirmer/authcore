# Task 2 — application

## Docs to READ (mandatory, at the pin)

- `auto-handlers` — the insert / update / patch handlers, their field contracts, and the
  in-transaction hooks.
- `command-handler` + `custom-command-handler` — the command shape, and where a command may
  read the request context.
- `auto-query-handlers` + `custom-query-handler` — the read's Result and its mapping, and the
  reserved read controls.
- `lifecycle-map` — what one write touches end to end, and how the audit verb is chosen
  (PATCH and PUT share a verb; the action name tells them apart).
- `query-side` — the criteria the query builds.

Convention: `conventions/application.md`. Read `task_children.md` too.

## What this layer contains

**Commands** — one per write operation of spec §5 and §3: create, patch, archive, plus the
two collection operations (attach a role, detach a role by entry id). No update-entry
command exists (`task_children.md`).

- **The command mappers are the only place allowed to read the request context**, and they
  are where the two runtime-only identity fields of the domain get populated — the requesting
  tenant from the identity's tenant claim, and the super-admin flag from the framework's
  sanctioned super-admin question. **Never** ask the caller-side permission check for the
  wildcard (it panics), and **never** hand-read the permissions claim: its name is
  configurable, so a hardcoded read starts answering false the day an operator renames it.
- Wire ids arrive as strings and convert to the framework id type at the mapper; the reverse
  at the response mapper.
- The patch mapper must not carry the handle or the tenant reference into the entity — they
  are frozen by G1 and G2, and the write shape excludes them (spec §8).
- The insert path assigns the tenant reference from the body, not from the claim: spec §10
  notes a super-admin creates inside a customer's tenant while carrying no tenant claim of
  their own. What stops an ordinary caller writing another tenant's reference is G5, not the
  absence of the field.

**Queries** — by id and by params, with the criteria of spec §9 and the **tenant isolation
filter** of spec §10 injected from the request context, skipped for a super-admin. A by-id
read of another tenant's row must come back as **not found**, not as forbidden — it leaks
nothing about who else exists.

**DTOs** — the collection entry's input, and the read row results. Granularity and naming
per `service-layout.html`.

**Translations** — all seven catalogs (pt-BR, English, Spanish, French, German, Italian,
Dutch), for the nine notifications and every field label. Real translations in all seven;
this is the one place in this repository where non-English text is allowed, and the
surrounding Go stays English.

## Acceptance check

- One command per operation of spec §5/§3; no update-entry command.
- The identity reaches the entity only through the command mappers, and only via the
  sanctioned super-admin question.
- The tenant filter is in the query criteria and the super-admin bypass is there with it.
- Seven catalogs, complete, for all nine notifications and every label.
- `gofmt -l` prints nothing; `go vet` and `go build` clean.
