# Spec: Tenant

Status: APPROVED
Approved: maintainer (Cláudio Schirmer Guedes), 2026-08-19 — every ⚠️ OPEN slot answered (§B Q1–Q10) and the remaining `(proposed)` picks accepted in one go
Language: English (all artifacts) · Portuguese (chat) — per `CLAUDE.md` rule 3 and the maintainer's invocation
Generation: <pending>

The tenant is the isolation partition every other aggregate of this service will hang off.
It carries **three identifiers, and each has exactly one job** — the single most important
thing to understand before reading anything else here:

| Identifier | Value | Who sees it |
|---|---|---|
| `id` | UUIDv7, minted by the framework | internal. Referenced by nothing; appears in the management API's URLs and nowhere else |
| `workspace` | `acme-comercio` | the human-facing handle — URLs, logs, support |
| `tenant_id` | `UUIDv5(namespace, workspace)` | **the public key.** The `tenant_id` claim of every token, and the FK target for every future aggregate |

`tenant_id` therefore means exactly one value everywhere it appears — column, token claim
and foreign key. That is deliberate: two earlier drafts of this spec had two different
values answering to that name, which is a defect that ships silently.

---

## 0. Inputs this spec was built from

| Input | What it contributed |
|---|---|
| Maintainer invocation | three fields (name, workspace identifier, description); "name ≥ 2 words"; "description ≥ 2 words, no keyboard junk"; "identifier per large-company practice" |
| Maintainer decisions at the model gate | §B — ten decisions, including the derived `tenant_id`, what the PK may be used for, the shared-vs-specific VO scope, the commercial status field, and the framework upgrade the archive rule required |
| `scaffold-service/spec.md` (APPROVED) | posture: Postgres SoR, **no Mongo**, no broker → relational-served views; REST + OpenAPI wired, GraphQL block present but inert |
| `README.md` | a **prior, already-reasoned Tenant model** from an earlier iteration — see the contradiction note below |
| Industry survey (Auth0, Microsoft Entra ID, Atlassian Cloud, Slack) | the workspace handle's shape and its mutability doctrine — §A |
| omnicore `v0.54.0` `/docs` **and source** | value objects, table schema, relational view capability, the `Loader.Exists` probe — plus the verified id-minting facts in §C |

### ⚠️ Discovery contradiction — surfaced, not resolved silently

`README.md` states the tenant registry is **built** (CRUD, archive/unarchive, REST +
GraphQL, permissions already gated) and documents a full field/rule model. **No such code
exists**: `internal/` is absent entirely and the service is the empty shell
`scaffold-service` produced. The README also pins omnicore `v0.51.0`; `go.mod` pinned
`v0.53.0` when this run started and `v0.54.0` after the upgrade taken mid-run (§C).

Reading: the README is a **stale forward-declaration** from an earlier attempt, not a
description of this working tree. It is treated here as a *prior recorded decision by the
same maintainer* — valuable input, aligned to wherever it does not conflict, with every
delta called out. It is **not** treated as existing code, so this is a `scaffold-entity` run
and not an `evolve-entity` one.

The README is now out of step with this spec on several points. **Bringing it up to date is
an explicit task of this run** (`tasks.md`, final task) and is done after the code exists,
so it describes reality rather than intent.

| Point | README (prior) | Settled here | Why |
|---|---|---|---|
| the handle's wire name | `slug` | **`workspace`** | maintainer — the product's word, not the dev's |
| handle regex | `^[a-z][a-z0-9]*(-[a-z0-9]+)*$` (leading letter) | `^[a-z0-9]+(-[a-z0-9]+)*$` | RFC 1123 relaxed RFC 1035's leading-letter rule; `3m` is a legitimate value |
| handle max length | 40 | **63** | the DNS-label ceiling every surveyed product uses; Auth0 is exactly 3–63 |
| `name` | 2–120 characters | 2–120 **+ anti-junk, no word count** | maintainer — §A.3 |
| what the JWT carries | `id` (the UUIDv7 PK) | **`tenant_id` = `UUIDv5(ns, workspace)`** | maintainer — §A.6. The PK is never issued to anyone |
| `name` uniqueness | unique among active tenants | **not unique** | §B Q8 — the README's own rule contradicted the market it was modeled on |
| commercial status | absent | **`status` = trial \| active \| suspended** | §B Q9 — suspension is not archiving (§5) |
| `description` | 15–500, ≥2 words, anti-junk, must differ | adopted as-is | agreed |
| Removal | archive only, no `DELETE` | adopted | §6 |
| Permissions | `tenant:read` / `:insert` / `:update` / `:archive` | adopted | §10 |

---

## A. The industry survey (the maintainer asked for it — it drives §2 and §7)

### A.1 — Everyone converges on a DNS label

| Product | The machine-facing tenant identifier | Shape | Mutable? |
|---|---|---|---|
| **Auth0** | tenant name → `<name>.auth0.com` | 3–63 chars · lowercase alphanumeric + `-` · may not begin or end with `-` · unique | **No.** Cannot be changed after creation, and cannot be reused once deleted |
| **Microsoft Entra ID** | initial domain → `<name>.onmicrosoft.com` | alphanumeric, no special characters | **No.** Permanent — cannot be altered or removed |
| **Atlassian Cloud** | site subdomain | ≥3 chars · lowercase letters, digits and hyphens, hyphens only in the middle | Yes, capped at 15 changes |
| **Slack** | workspace URL | letters, digits and hyphens | Yes |

The shape is the same everywhere because the value lands in a hostname or a URL path: it is
a **DNS label** — letters, digits and hyphens, no leading or trailing hyphen, at most 63
octets. Nothing here is a taste decision; it is the constraint the transport already
imposes. (RFC 1035 additionally required a leading letter; RFC 1123 relaxed that, which is
why `3m` is accepted here and the README's prior regex is widened.)

### A.2 — Two doctrines on mutability, and only one fits this service

- **Identity providers (Auth0, Entra) freeze it forever.** The value is a *durable
  reference*: it appears in issuer URLs, in SAML/OIDC metadata, in bookmarked login pages,
  in audit trails and in every partner's configuration.
- **Collaboration products (Slack, Atlassian) let it change**, because there the value is a
  convenience URL and the product runs a redirect for the old one.

`authcore` is an identity provider, so the Auth0/Entra doctrine applies. **And under §A.6 it
stops being a doctrine and becomes arithmetic:** `tenant_id = UUIDv5(namespace, workspace)`
is a pure function, so re-issuing a workspace re-issues *the identical `tenant_id`*. A
recycled workspace does not merely confuse a bookmark — it mints a tenant whose public key
is byte-identical to the archived one's, and every token ever issued for the old tenant
authorizes against the new one's data. Valid signature, correct claim, wrong tenant, no
anomaly to detect.

That is why rules 3–7 are not stylistic:

- **immutable** (rule 7) — a changed workspace changes `tenant_id`, orphaning every live
  token and every downstream foreign key at once;
- **never reused, archived rows included** (rule 5) — the collision above;
- **format-bounded and reserved-list checked** (rules 3–4) — the handle reaches logs, URLs
  and other services' parsers, so its alphabet is part of the contract.

### A.3 — Where the survey did *not* support the request: "name ≥ 2 words"

No surveyed product imposes a word count on the tenant's human-readable name. The reason is
concrete: single-word corporate names are ordinary — Nubank, Stone, Ambev, IBM, Google,
Petrobras — and a two-word rule rejects them at registration.

The rule the request was reaching for is **substance**, not word count. Word count is a good
proxy for substance in a *description* (a one-word description really is junk) and a poor
one in a *name*. **Settled: `name` is the display name and carries the anti-junk heuristics
without a word count; `description` keeps the two-word rule.**

### A.4 — What every subdomain-per-tenant product also does, and the request did not mention

**A reserved-handle blocklist.** If the handle reaches a hostname or a first path segment,
values like `www`, `api`, `admin`, `auth`, `login`, `mail`, `status` collide with platform
infrastructure. Grabbing `admin` is also a cheap phishing surface. This service has concrete
collisions already in its own routes: `docs`, `openapi.json`, `graphql`, `livez`, `readyz`.
Adopted as rule 5.

### A.5 — Honest limit of anti-junk heuristics

They raise the cost of garbage; they do not prevent it. `asdf asdf` passes any rule that
counts words and distinct characters. The heuristics catch the *lazy* case — a held key, a
single character repeated, the name pasted into the description — and nothing more. The real
defenses against a determined actor are rate limiting and operator review, which are not
this entity's job. Recorded so the rules are not mistaken for a guarantee.

### A.6 — Why the token carries a derived UUID and not the PK

The maintainer's call, and the reasoning is worth keeping because it is not the obvious one.

**Why not the PK.** `tenants.id` is a **UUIDv7**, and UUIDv7 embeds a millisecond timestamp
by construction. Issuing it as a claim would hand every client and every consuming service
the exact creation instant and the creation ORDER of every tenant — how many customers
signed in March, who was first. Mild, but it is business information leaving for free and
it cannot be recalled once tokens are in the wild.

**Why derive rather than mint a second random id.** A derived value is *recomputable*: any
service that knows `acme-comercio` reaches the same `tenant_id` offline, with no lookup and
no call back to authcore. A second random UUID would need a round-trip or a cache.

**Why the obvious objection does not apply.** UUIDv5 is `SHA-1(namespace ‖ name)` — given
the namespace, `tenant_id` is brute-forceable back to `workspace` over a small dictionary of
company handles. It is therefore **not a secret and must never be treated as one**. That
costs nothing *here*, because the thing it fails to hide — the workspace — is public by
design: it is in the URL, in the logs, on the login page. What the derivation does hide is
the creation timestamp, and that it hides completely.

**The trade accepted:** a claim of 16 bytes instead of a 63-char string, at the cost of a
value a human cannot read in a log without resolving it. Token issuance is not built yet;
this is recorded as the constraint that entity must honor.

---

## B. Decisions taken at the model gate (no longer open)

| # | Question | Answer |
|---|---|---|
| Q1 | Is `name` the legal name or the display name? | **Display name** — anti-junk heuristics, **no word count** |
| Q2 | What is the handle called on the wire? | **`workspace`**. Rejected: `identity`, which `README.md` already reserves for the future person-credential aggregate — one word, two concepts is the defect being avoided |
| Q3 | Which surfaces? | **REST + OpenAPI and GraphQL.** No CSV/XLSX exports |
| Q4 | Data-access (Layer 2/3)? | **Anyone holding the permission sees and edits every row** — no ctx row filter |
| Q5 | What does the JWT carry? | **`tenant_id`, a UUID derived from `workspace`** — not the PK. §A.6 |
| Q6 | By-id routes expose the PK in the URL. Accept? | **Accepted.** The goal is the PK staying out of tokens and out of consuming services, which is where the leak would scale. The management API is operator-only and already behind a permission. Keeps the framework's automatic by-id handlers — the alternative was custom query + command handlers resolving `tenant_id` → PK on every write |
| Q7 | Shared VOs or entity-specific ones? | **`vos.DisplayName` and `vos.Description` are shared** (Tenant now, `Group`/`Role` later); **`vos.TenantWorkspace` stays specific** — it carries the reserved list and the derivation. Anti-junk predicates extracted as pure helpers. Line drawn: `DisplayName` is for things, so `User` will get its own `PersonName`. Full reasoning in §2 |
| Q8 | Is `name` unique? | **No.** Reversed from an earlier draft, which contradicted this spec's own survey — no surveyed product makes the display name unique, only the handle. Removes rule 3, the partial index, the `IfUnarchive` re-check and the unarchive 409 |
| Q9 | A commercial status field? | **Yes, now: `vos.TenantStatus` = `trial` \| `active` \| `suspended`** (§7 rules 11–12). Moves through the ordinary `PATCH` under `tenant:update`, not through dedicated intent routes — the two costs of that are recorded in §10. **Mandatory on insert, no server-side default** |
| Q10 | Archiving and status | **Archiving forces `suspended`** (rule 13). Reaching it required upgrading the framework mid-run, `v0.53.0` → `v0.54.0`, because at the old pin the mutation reached the audit event and never the row — §C. Rejected alternatives: requiring `suspended` before archive (an `IfArchive` validation), a hand-written lifecycle hook, deriving it on read |

**Recorded inference, correctable in one word:** Q5's chosen option stated that the PK
becomes "a surrogate nothing references". Taken at face value, that means the future
`users.tenant_id` (and `groups`, `roles`) reference **`tenants.tenant_id`**, not
`tenants.id`. That is what makes `tenant_id` a single unambiguous value across column,
claim and FK, and it is how this spec records it.

---

## C. Verified framework facts that shaped §7 (read the source, do not re-derive)

The derivation was originally proposed as "compute it inside `IfInsert`". **That does not
work, and it fails silently** — which is why it was checked before being written down.

| Fact | Evidence at the pin (`v0.54.0`) |
|---|---|
| The framework already derives ids this way — but **only for a shared base** | `infra/db/command/write/shared_base_write.go:57` → `deterministicBaseID(v)` = `uuid.NewSHA1(sharedBaseNamespace, []byte(v))`, reached only from shared-base write paths |
| `NaturalID(col)` is declarable on any `TableSchema`, but only the shared-base writer consumes it | `infra/db/core/shared_base.go:81`; no flat-path reader of `naturalIDCol` |
| **The flat insert mints its own id unconditionally** | `infra/db/command/write/flat_write.go:35` → `id, err := newWriteID()` (`uuid.NewV7()`), with no check of any pre-set entity id |
| **The handler then overwrites the entity's id with the minted one** | `application/handlers/insert.go:67` → `entity.SetID(id)` after `Repo.Insert` returns |
| By-id routes bind `:id` to the PK | `web/spec_query.go:60`, `web/spec_command.go:40`,`:104` — `HasPathID: true` |
| **Archive persists the entity's full field set**, so a mutation made in an `IfArchive` closure reaches the row | `infra/db/command/write/flat_write.go` → `Archive` delegates to `softWrite`, which calls `schema.WriteFields(src)` + `buildUpdate(...)`. **This is new in v0.54.0** — see the note below |

**Consequences, applied throughout this spec:**

1. A derived id assigned in `IfInsert` or in `ToEntity` is discarded without a warning.
   `tenant_id` is therefore an **ordinary persisted column**, not the PK.
2. Making the derived value the actual PK is reachable only by modeling Tenant as a shared
   base — two tables for a single role, contradicting §1. **Rejected**: buying a mechanism
   with a wrong model is a bad trade.
3. The derivation runs in the insert command's `ToEntity` — the application-layer mapper —
   never in `BuildRules`, which is a validation pass and may run more than once.

### The one fact that changed under this run — and why the pin moved

This spec was first written against **v0.53.0**, where archive was the framework's last
write-path exception: `UPDATE <table> SET deleted_at = $1, revision = revision + 1 WHERE
id = $2` and nothing else (`write_sql.go:118` at that pin). A rule like
`r.IfArchive(func(){ t.Status = Suspended })` therefore mutated the entity, reached the
audit event and the outbox payload — both built from `schema.WriteFields(src)`, i.e. from
memory — and **never reached the row**. Not a no-op: a divergence between the row and the
event stream, with nothing to detect it.

**v0.54.0 removes that exception** (released 2026-08-19, upgraded to in this run):
archive and unarchive now emit the same UPDATE every other verb emits — full field set,
managed timestamps, revision bump, revision guard — with the archive transition riding
along as one more written column. Its changelog names this exact rule shape as the
motivation. Rule 13 below is therefore a plain `IfArchive` closure with no supporting
infrastructure.

**Every OTHER fact in the table above was re-verified against v0.54.0 and is unchanged**,
so the derived-`tenant_id` design stands exactly as written.

### The namespace constant

```
TenantIDNamespace = e2937874-80cb-4b5f-b113-21741931ac1a
```

Generated once, for this service. It lives as an exported constant beside the value object.

> **It must never change.** Changing it re-derives every `tenant_id` in existence, which
> invalidates every issued token and every foreign key pointing at one, with no migration
> path short of reissuing the whole platform's tokens. It is deliberately a project
> constant and not configuration, so it cannot be changed by a deployment.

A service-specific namespace (rather than a standard DNS/URL namespace) is used so that no
other system deriving from the same handle can produce a colliding value.

---

## D. Where this model meets its wall at enterprise scale

Written because the maintainer asked whether anything looks odd when the model is held up
against how large companies actually work. Two of the findings changed the spec (§B Q8 and
Q9). The four below did **not** — they are recorded so the wall is a known location rather
than a surprise, in the same spirit as the `Identity`/`User` escape hatch already in
`README.md`.

### D.1 — One level, where the market has two

Every product surveyed in §A has **two** levels above the user, and this model has one:

| Product | Outer | Inner |
|---|---|---|
| **Auth0** | tenant = an *environment* (dev/staging/prod) | Organization = the customer |
| **Slack** | Enterprise Grid Org | Workspace (per department, per region) |
| **Atlassian** | Organization | Site |

Auth0's split is the counter-intuitive one worth knowing: **their "tenant" is not the
customer at all** — it is the environment, and the customer lives inside it. Our `Tenant`
conflates both roles.

The request that does not fit: *"one workspace per subsidiary, one invoice."* Nothing here
answers it today.

**Not built, deliberately** — a second level is real modeling work (a parent aggregate,
a nullable parent reference, scoping rules on every query) for a customer shape that does
not exist yet. **The escape hatch:** a `TenantGroup` aggregate with `Tenant` gaining an
optional reference to it. That is additive — no existing column moves, no `tenant_id`
changes, no token reissue — which is exactly why it can wait.

### D.2 — A rebrand has no answer here, and the market's answer is not "allow the change"

`workspace` is immutable and `tenant_id` derives from it, so a customer that renames itself
(acquisition, rebrand — `twitter` → `x`) keeps the old handle in its URLs permanently.

Auth0 and Entra carry the identical restriction, and their release valve is **not**
permitting the change: it is a **custom domain**, where the customer points their own
hostname at the service and the frozen handle stops being visible.

**The rule that follows, and it is not negotiable:** the answer to a rebrand is a display
alias or a custom domain — **never** editing the workspace. Editing it changes `tenant_id`,
which invalidates every issued token and orphans every foreign key at once (§A.2). A future
`displayDomain` field would be additive and would not touch the derivation.

### D.3 — The anti-junk rules assume a human is typing

At scale, tenants are not created by people. They are created by a provisioning system when
a contract is signed. A mandatory 15-character description with two words and no repeated
runs means that system will emit `"Tenant for Acme Corp"` forever — passing every rule and
meaning nothing.

**Kept anyway**, with eyes open: the rules protect the manual and self-serve paths, and are
ceremony on the automated one. They cost the automated path nothing but a constant string.
Worth revisiting only if machine-created tenants become the dominant path.

### D.4 — Archive-only and the right to erasure

No `DELETE` verb exists (§6), so a tenant row is never purged. Under GDPR-style erasure
obligations that would normally be a gap.

It is not one **here**, and the reason should be stated rather than assumed: a `tenants` row
holds a company's name, handle and description — no personal data. Erasure obligations
attach to `User`, and that is where a purge path will have to be designed. `Tenant` must
survive precisely so the erased user's foreign keys and audit trail stay coherent.

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** (proposed; alternative: sharedbase-role — rejected in §C.2)

  No identity smell. The identity-smell test asks whether the field set carries a real-world
  party/asset identity *plus* role-specific fields — an identity **playing a role**. A
  tenant is not a party playing a role; it *is* the partition other aggregates are scoped
  by. There is no second role a tenant could also become for the same underlying identity: a
  company buying two products on this platform is still one tenant.

  (Independently: the `SharedBaseView` read kind would need Mongo, which this service does
  not have. That is **not** the reason for this pick — the posture never constrains
  write-side modeling — but nothing was lost.)

- **ER sketch**

  ```
  tenants  (the isolation partitions of the platform; one row per customer organization)
    id           UUID        PK    — UUIDv7, framework-minted. Referenced by nothing;
                                     appears only in the management API's by-id URLs
    tenant_id    UUID        NOT NULL, UNIQUE  — UUIDv5(namespace, workspace).
                                     The public key: the `tenant_id` token claim and the
                                     FK target of every future aggregate
    name         TEXT        NOT NULL           — NOT unique; see §7 and §B Q8
    workspace    TEXT        NOT NULL, UNIQUE across ALL rows, active and archived alike
                                     (plain unique index)
    description  TEXT        NOT NULL
    status       TEXT        NOT NULL           — trial | active | suspended.
                                     Membership is enforced by the enum VO, not by a CHECK
    created_at   TIMESTAMPTZ NOT NULL   — managed
    updated_at   TIMESTAMPTZ NOT NULL   — managed
    deleted_at   TIMESTAMPTZ NULL       — managed; NULL = active
    revision     managed
  ```

  One table. No child tables, no sibling tables, no base table.

  Table COMMENT: *"Isolation partitions of the platform. tenant_id is the public key —
  derived from workspace, issued as the tenant_id token claim, and the target of every
  foreign key; the id column is a framework surrogate and is never issued to anyone."*

- If sharedbase-role: **N/A — flat.**

---

## 2. Fields

| Field | Go type | VO? | Nullable | Unique | Lives on | `example:` | Description |
|---|---|---|---|---|---|---|---|
| `ID` | `*domain.ID` | plain (managed) | no | PK | root | `0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410` | Framework-minted surrogate key. Not issued in tokens and referenced by no foreign key. |
| `TenantID` | `domain.ID` | plain (derived) | no | **yes — across all rows** | root | `a3f1c07e-2b58-5d94-8e61-4f2093ab77d5` | Public key of the tenant, derived from `workspace`. Issued as the `tenant_id` claim of every token and referenced by every tenant-scoped aggregate. **Read-only on every surface** — never a member of an insert or update DTO (§9). |
| `Name` | `vos.DisplayName` (over `string`) | **new-raw, SHARED** | no | **no** | root | `Acme Comércio e Serviços Ltda` | Human-readable display name of the tenant organization, as operators and end users see it. |
| `Workspace` | `vos.TenantWorkspace` (over `string`) | **new-raw, entity-specific** | no | **yes — across all rows, active and archived** | root | `acme-comercio` | Immutable handle of the tenant; reaches URLs, logs and external configuration, and is the input the public `tenant_id` is derived from. |
| `Description` | `vos.Description` (over `string`) | **new-raw, SHARED** | no | no | root | `Retail operations of the Acme group in Brazil.` | What this tenant is, in the platform operators' own words. |
| `Status` | `vos.TenantStatus` (over `string`) | **new-enum, entity-specific** | no | no | root | `active` | Commercial lifecycle of the tenant: `trial`, `active` or `suspended`. Orthogonal to archiving — a suspended tenant is still listed and still authenticates for billing. |

Managed columns (`CreatedAt`, `UpdatedAt`, `DeletedAt`, `Revision`) are declared by presence
on the schema; they are not modeled as domain fields.

For `name`, `workspace`, `description` and `status` the wire name, the Go field name and
the column name are the same word. Nothing is renamed at a boundary.

**Notes on the `VO?` column** — the classification is deliberate, not reflex:

- The three text fields carry bespoke **format / length / composition** rules. Per
  `value-objects.html` that is the raw-VO kind: the rule lives in one place, is validated
  automatically on **every** write (insert, update, archive, unarchive), and is unit-tested
  once instead of per mode. Leaving these as plain `string` with the checks inline in
  `BuildRules` is exactly the smell the final verify greps for.
- A raw VO also answers an empty value with `RequiredFieldNotification` from its own
  `IsValid`. **So `required` is NOT declared again in `BuildRules`** — doing so makes the
  caller read the same complaint twice for one empty field.
- `TenantID` is **plain, not a VO**: it holds no rule of its own. Its only invariant is
  agreement with `Workspace`, which is a cross-field rule (rule 8) and belongs in
  `BuildRules`, not in a value object. Its Go type is `domain.ID`, which the pin maps to a
  native Postgres `UUID` column (`shared/dialects/postgres.md`).
- The derivation itself is a **method on `vos.TenantWorkspace`** —
  `DeriveTenantID() domain.ID` — so the rule that ties handle to public key lives with the
  handle and is unit-tested there. See §C for where it is called.
- `Status` is the **enum kind**, string-backed so the persisted token stays readable in the
  database and stable on the wire: members `trial` / `active` / `suspended`, with the zero
  value `""` as the Unknown sentinel that is never a member. It declares `Value()`,
  `Values()` and `UnknownNotification()` and writes **no** `IsValid` — membership is checked
  by the framework. Wire values are parsed with `domain.EnumByValue[vos.TenantStatus]` at
  the mapper, so anything unrecognized converges to Unknown and is rejected (422) rather
  than persisted. Per-locale labels come from `domain.EnumDescriptionKey` →
  `TenantStatus.trial` etc., registered in all seven catalogs.
- **The column is `TEXT NOT NULL` with no `CHECK` constraint** (low-risk, stated). Membership
  is a domain invariant, and reconstruction converges an unrecognized stored value to the
  Unknown sentinel rather than inventing a phantom member. A `CHECK` would add a migration
  to every future member for a case the write path cannot produce.
- **No composite.** No two of these fields mean something only together. `Workspace` and
  `TenantID` come close, but one is *derived from* the other rather than *paired with* it —
  a composite would store both under one type and lose the derivation.

### VO scope — shared vs entity-specific (decided; sets the precedent for the whole system)

An earlier draft made all three VOs entity-specific. Re-examined rule by rule at the
maintainer's prompting, that was wrong for two of them:

| Field | Rules that are *about the tenant* | Verdict |
|---|---|---|
| `Name` | none — every rule says "human-typed text, not junk"; only the 2–120 **bound** is a calibration | **share** |
| `Description` | none — same | **share** |
| `Workspace` | the reserved list (this service's own routes) and `DeriveTenantID()` | **keep specific** |

**The objection that dissolved.** The first draft argued a shared VO would produce a
generic error message. It does not: per `value-objects.html`, *"the label of a field lives
on the aggregate's struct tag (`labelKey:…`), next to the field — never inside the value
object. The framework resolves it at emit and stamps it on the notification."* A shared
`vos.Description` emits a shared notification, and the **holder's `labelKey` supplies which
field failed** — so the 422 still names the tenant's description, with the VO knowing
nothing about tenants.

**The layout: primitives separate from types.** The anti-junk predicates (no run of N
identical characters, at least N distinct, word count, has a vowel, trimmed and
single-spaced) are pure helpers written and tested once; each VO composes them with its own
bounds and emits its own notification. Their exact Unicode/rune semantics are pinned in §7
("How the text predicates are defined") and are binding on the helpers, not on each VO.

- `vos.DisplayName` — 2–120. Tenant, and later `Group` / `Role`.
- `vos.Description` — 15–500. Tenant, and later `Group` / `Role`.
- `vos.TenantWorkspace` — 3–63, plus the reserved list and `DeriveTenantID()`.

**The line drawn now, so sharing does not sprawl: `DisplayName` is for THINGS, not people.**
When `User` arrives it gets its own `vos.PersonName` — a person's name follows different
cultural norms, and a change made for it must not silently move the tenant's bounds.

**The two costs, accepted knowingly:**

1. *Schedule coupling.* Changing a shared bound moves it for every holder at once — and the
   DDL of each holder's table may not follow. **Changing a shared VO is a change to every
   entity that carries it, and must be reviewed as one.**
2. *A rule only one holder needs does not fit.* Then that holder splits off its own type.
   Entering the sharing is cheap and leaving it is cheap — one type rename plus one file —
   which is what makes it worth doing now rather than debating.

---

## 3. Children (1:N)

**N/A — no collections.** `User`, `Group` and `Role` will each be their own root aggregate
holding a `tenant_id` reference (to `tenants.tenant_id` — see §B), per the target model
recorded in `README.md`. Modeling them as children of `Tenant` would mean a user could only
ever be loaded through its tenant and could not be archived on its own — the "restorable
alone ⇒ own aggregate" test answers this in one line.

---

## 4. Siblings (1:1)

**N/A — no facet worth splitting, and no optional field at all.** All business fields are
required, so the optional/sparse/bulky trade-off a sibling answers does not arise. This also
keeps §8 free to be PATCH-only (a sibling would force PUT into the shape).

---

## 5. Modes                                             [required]

**`Display, Insert, Update, Archive, Unarchive`** (proposed; alternative: add `Delete`,
rejected in §6 — or drop `Update`, the "freeze-once" pattern, wrong here because a tenant
legitimately renames itself).

`Archive`/`Unarchive` in the set ⟺ the schema declares `DeletedAt("deleted_at")` ⟺ the
migration carries that column. The three move together or the boot panics.

**`Status` is orthogonal to the mode set and must not be confused with it.** Archiving is
*removal* — the row leaves every default read. Suspension is *commercial state* — the row
stays listed, stays readable, and keeps authenticating for billing while being blocked in
the product. Using archive to suspend a delinquent customer would hide them from the very
reports and screens the collections process needs. They compose, but not freely in both directions: archiving **forces** `suspended`
(rule 13), so an archived tenant is always suspended, and unarchiving brings it back
suspended. The reverse does not hold — activating a tenant does not unarchive it.

---

## 6. Delete semantics                                  [required]

**Soft only — archive/unarchive. No `DELETE` verb is generated** (proposed; alternative:
both verbs).

The reason is structural. `tenants.tenant_id` will be a foreign key target for `users`, and
the same value is the `tenant_id` claim inside tokens already issued and still valid. An
irreversible purge orphans both. Worse, it would erase the row that *reserves* the
workspace — and since `tenant_id` is a pure function of the workspace (§A.2), the next
registration taking that handle would be issued the **identical public key**. Archiving
keeps the row addressable for audit, for the FK and for the reservation, while removing it
from every default read.

Verb truth, honored: `PATCH /tenants/{id}/archive` and `PATCH /tenants/{id}/unarchive`.
Nothing soft is ever wired behind `DELETE`.

**What archiving actually executes at this pin (v0.54.0):** the same UPDATE every other
verb emits — the full field set, `updated_at` stamped, `revision` bumped and guarded —
with `deleted_at` bound to the operation's instant as one more written column. Three
consequences that reach the API contract: archiving a tenant **stamps `updated_at`** (it
is a mutation, and it surfaces in "changed recently" listings); archiving a row that is
not there answers **404** instead of committing an event about nothing; and archive can
answer **409** like any other guarded root update (§9).

Per-child: **N/A — no children.**

---

## 7. Business rules                        [required]

Legend — **auto (VO)** means the rule lives in the value object's `IsValid` and the framework
runs it on every write with no `BuildRules` entry; the other scopes are `BuildRules` mode
gates.

| # | Field(s) | Rule | Scope | Notification | HTTP |
|---|---|---|---|---|---|
| 1 | `Name` | required (non-empty) | auto (VO) | `RequiredFieldNotification` | 422 |
| 2 | `Name` | 2–120 characters · at least one letter · **distinct characters ≥ min(3, length)** · no run of 4 or more identical characters · no leading/trailing whitespace and no internal double space · **no word count** | auto (VO) | `InvalidDisplayNameNotification` | 422 |
| 3 | `Workspace` | required · 3–63 characters · `^[a-z0-9]+(-[a-z0-9]+)*$` (lowercase alphanumerics and hyphens; never leading, trailing or doubled) · at least 3 distinct characters · no run of 4 or more identical characters | auto (VO) | `RequiredFieldNotification` / `InvalidTenantWorkspaceNotification` | 422 |
| 4 | `Workspace` | not in the reserved list below — §A.4 | auto (VO) | `ReservedTenantWorkspaceNotification` | 422 |
| 5 | `Workspace` | **unique across ALL rows, active and archived alike** — never released. A plain `UNIQUE` over the column, **not** a partial index: an archived remnant *must* block a new tenant taking that handle, because the handle derives the public key (§A.2) | `IfInsert` | `TenantWorkspaceAlreadyExistsNotification` | 409 |
| 6 | `TenantID` | **unique across ALL rows.** Structurally implied by rule 5 (a pure function of a unique input is unique), so the index is a backstop and the lookup path, not the primary guarantee | `IfInsert` | `TenantIDAlreadyExistsNotification` | 409 |
| 7 | `Workspace`, `TenantID` | **both immutable after creation.** Structural first: neither is a member of the update DTO, and `TenantID` is not a member of the insert DTO either — it is derived, never proposed by a caller (§9). The `domain.Old(e)` guard is the belt-and-braces layer (proposed; alternative: structural only). **One notification per field, emitted on the field that was violated** — a single shared one would tell a caller that "the workspace is immutable" when `tenant_id` was the field that moved | `IfUpdate` | `TenantWorkspaceIsImmutableNotification` / `TenantIDIsImmutableNotification` | 422 |
| 8 | `TenantID` vs `Workspace` | `TenantID` **must equal** `Workspace.DeriveTenantID()`. The value is computed by the mapper, so this can only fail through a future mapper bug or a hand-written row — which is exactly what it is for | `IfInsertOrUpdate` | `TenantIDDerivationMismatchNotification` | 422 |
| 9 | `Description` | required · 15–500 characters · **at least two words** (a word = a run of 2 or more letters) · at least 5 distinct characters · no run of 4 or more identical characters · at least one vowel | auto (VO) | `RequiredFieldNotification` / `InvalidDescriptionNotification` | 422 |
| 10 | `Description` vs `Name` / `Workspace` | must differ from both under a normalized comparison (case-folded, whitespace- and hyphen-collapsed) — catches the pasted-name description | `IfInsertOrUpdate` | `TenantDescriptionMustDifferNotification` | 422 |
| 11 | `Status` | must be a declared member — `trial`, `active` or `suspended`. **Mandatory on insert with no default** (§B Q9): an absent or unrecognized value converges to the Unknown sentinel and is rejected. **On insert ANY member is accepted, `suspended` included** — rule 12 gates transitions, not creation, and a data migration must be able to land a delinquent tenant in the state it was already in. **No separate `required` rule is declared** — the enum's own unknown-member notification already answers an empty value | auto (VO, enum) | `UnknownTenantStatusNotification` | 422 |
| 12 | `Status` | **allowed transitions only.** `trial → active` · `trial → suspended` · `active → suspended` · `suspended → active` · and any no-op (same value). Everything else is refused — notably `active → trial` and `suspended → trial`: a trial is a beginning, never something a tenant returns to. Compared against `domain.Old(e).Status`, which is nil on insert, so the rule is `IfUpdate` only | `IfUpdate` | `InvalidTenantStatusTransitionNotification` | 422 |
| 13 | `Status` | **archiving forces `suspended`** — set in an `IfArchive` closure, not validated. An archived tenant is never commercially active, so `archived + active` becomes an unrepresentable state. Requires v0.54.0: at v0.53.0 this mutation reached the audit event and never the row (§C) | `IfArchive` | — (a mutation, not a validation) | — |

### How the text predicates are defined — binding, not stylistic

The three text VOs share the anti-junk helpers, so these definitions are written once and
apply to every rule above that names them. Both exist because the obvious Go implementation
is wrong for this service.

- **Everything counts RUNES, never bytes.** Every length bound (2–120, 3–63, 15–500) and
  every distinct-character count is over `[]rune`, not `len(s)`. `"Acme Comércio e Serviços
  Ltda"` is 29 runes and 31 bytes — a byte-based bound is wrong by 2 for the exact naming
  style this service exists to serve, and wrong by more for every accented Portuguese name
  after it. Same for "run of 4 or more identical characters": identical *runes*.
- **"At least one vowel" is defined over Unicode, not `[aeiou]`.** A rune counts as a vowel
  if it is a Latin vowel including its accented forms (`a e i o u á é í ó ú ã õ â ê ô à ü`
  and their uppercase), **or** if it is any letter outside the Latin script. This service
  ships seven translation catalogs; an ASCII-only vowel test would reject a description
  written in Japanese, Arabic, Chinese, Russian or Hebrew **as keyboard junk**. The rule is
  kept because it still costs a Latin-script masher something (`zzz xxx` fails), and it
  must never cost a non-Latin writer anything.
- **"A letter" and "a word" are Unicode too** — `unicode.IsLetter`, so a word is a run of
  2 or more letters in any script.

These three definitions are exactly what the shared helpers' unit tests must pin, with at
least one non-Latin and one accented-Latin case each.

**Why `distinct ≥ min(3, length)` on the name rather than a flat "≥ 3 distinct".** A flat
rule would be unsatisfiable below 3 characters and would reject `3M` and `GE`, which are
real display names. The formula reads as one sentence: *at least 3 distinct characters, or
as many distinct characters as the value is long when it is shorter than that.* `3M` passes
(2 of 2), `aa` fails (1 of 2), `aaaa` fails on the run rule too.

**`Name` is NOT unique — deliberately** (§B Q8; reversed from an earlier draft of this
spec). No surveyed product makes the tenant's display name unique; only the handle is. Two
genuinely different customers named "Acme" are ordinary, and rejecting the second forces a
real company to register under a name that is not its own. The operator's need to tell rows
apart is already served by `workspace`, which sits beside the name in every listing. Three
things fall away with it: the partial unique index on `name`, the `IfUnarchive` re-check,
and the 409 that an unarchive could previously answer.

**Rule 14 — no silent normalization** (proposed; alternative: trim and lowercase on input).
`" Acme "` and `"ACME-CORP"` are **refused**, never quietly repaired. For a handle that is
immutable, reserved forever and feeds a derived public key, storing something the caller did
not send is worse than a 422: the caller believes they own `ACME-CORP`, every log line says
`acme-corp`, the derived `tenant_id` is computed from a value they never typed, and there is
no second chance to correct it. The same discipline is applied to the name so the API's
contract is consistent.

**Rule 15 — the workspace is explicit, never derived from the name.** The server does not
synthesize one when it is omitted. Deriving an immutable, permanently reserved handle from a
mutable field is a trap: the derived value is wrong the first time the name changes, and by
then it cannot be fixed. A UI is free to *suggest* one; the API requires it.

**Reserved workspace list** (low-risk — edit freely; one Go slice and one test):

```
admin · administrator · api · app · apps · assets · auth · billing · cdn · console
dashboard · dev · docs · files · ftp · graphql · help · host · id · internal · livez
login · logout · mail · media · metrics · new · oauth · openapi · platform · public
readyz · register · root · security · settings · signin · signup · smtp · sso · staging
static · status · support · system · tenant · tenants · test · user · users · www
```

`docs`, `graphql`, `livez`, `readyz`, `openapi` are not hypothetical — they are routes this
service serves today. `tenant`/`tenants` reserves the REST collection segment.

**Uniqueness enforcement style — the recommended chain, for rule 5:** a
`domain.Service` pre-check in `BuildRules` using the loader's hydration-free
`Exists(ctx, criteria)` probe (`custom-command-handler.html`), **plus** the database index
and the repository `Constraints` binding as the race-window backstop. The pre-check is what
lets a duplicate be reported *together* with the other validation errors instead of alone
after everything else passes; the index is what makes the invariant true under concurrency.
(Alternative: constraint-only — simpler, one fewer moving part, worse error reporting.)
Rule 6 is constraint-only: its pre-check would be a redundant probe of the same fact rule 5
already established.

**Three consequences of rule 13, stated so none is discovered later:**

1. **Rule 12 is never violated by it.** Every transition rule 13 can cause — `trial →
   suspended`, `active → suspended`, `suspended → suspended` — is already allowed. And it
   could not conflict anyway: rule 12 is `IfUpdate`, rule 13 is `IfArchive`, and the two
   modes never run together.
2. **Unarchiving returns the tenant `suspended`.** Nothing sets it back — the stored value
   *is* `suspended` because archiving wrote it. That is the wanted behavior, not an
   oversight: restoring a tenant must never silently resume a billable, functioning
   account. Reactivation is always a separate, explicit, separately audited act.
3. **It is one-way.** Setting a tenant `active` does NOT unarchive it. Archiving is
   removal and status is commercial state; only the first implies anything about the
   second.

`Workspace` is the only field carrying a uniqueness probe, so the domain Service exists for
exactly one question. That is still enough to require it:

⇒ `RequiresService() bool { return true }`. Every write handler must therefore be wired with
`Service:` — a nil there is a runtime failure `go build` cannot catch.

---

## 8. Update shape                                      [required]

**PATCH only** (proposed; alternative: PUT, or both). No sibling exists, so the PUT
invariant does not apply. **The updatable set is `name`, `description` and `status` —
nothing else.**

PATCH expresses the model honestly: `workspace` and `tenant_id` are immutable and simply
are not members of the update DTO, which is what makes rule 8 structural rather than
hopeful. `tenant_id` goes further and is absent from the INSERT DTO too — it is derived,
so there is no surface on which a caller proposes it (§9).

---

## 9. Surfaces & reads

- **REST + OpenAPI: yes** — these operations and nothing more. `{id}` is the PK
  (§B, Q6):

  | Operation | Route |
  |---|---|
  | list / filter / paginate | `GET /tenants/` |
  | create | `POST /tenants/` |
  | read one | `GET /tenants/{id}` |
  | partial update (name, description) | `PATCH /tenants/{id}` |
  | archive | `PATCH /tenants/{id}/archive` |
  | unarchive | `PATCH /tenants/{id}/unarchive` |

  **`tenant_id` is READ-ONLY on every surface — it is not a member of any write request
  DTO.** The insert body carries `name`, `workspace`, `description` and `status` (all
  four mandatory — `status` has no server-side default, §B Q9); the update body carries
  `name`, `description` and `status`. `tenant_id` appears in neither, so it is absent from
  the OpenAPI request schemas and from the GraphQL input types, and there is no shape in
  which a caller can propose a value for it. It is derived from `workspace` by the insert
  mapper and never assigned again.

  Not defending the field is the point: a value the contract never accepts needs no
  guard at the boundary, no "ignore it if present" branch, and no test for a caller
  trying. The only path that can produce a wrong `tenant_id` is a bug in the mapper
  itself, which is exactly what rule 8 exists to catch.

  On the read side it is fully present: every response carries it, and it is filterable
  (below) — that is how a consuming service resolves a token claim back to a tenant.

- **GraphQL: yes**. This entity's feature implements `bootstrap.GraphQLFeature`, which is
  what brings the declared-but-unmounted surface up for the first time: queries for the
  by-id and list reads, mutations for insert / update / archive / unarchive. Authorization
  is per registration unit — the same permission strings as REST, applied at each field.
- **Exports (CSV/XLSX): no.**
- **gRPC: no** — additive later via `/omnicore:implement`, no rework.
- **Integration events: not available on this posture** (publishing rides the CDC relay,
  which does not exist here — `scaffold-service/spec.md`). Noted so it is not lost.

- **Optimistic concurrency — every write can answer 409.** New at v0.54.0 and part of this
  entity's contract, not an implementation detail: every root update pins the revision it
  was loaded with in its own `WHERE`, so a write built on a stale read matches zero rows
  and is **refused** with `ConcurrentModificationNotification` (409) instead of silently
  reverting another writer's columns. This covers `PATCH`, `archive` and `unarchive`
  alike. The caller's recovery is to reload and reapply. It is free on the happy path —
  the guard rides the `WHERE` the statement already had; only the failure path pays one
  `SELECT 1`, to tell "the row is gone" (404) from "the row moved" (409).

- **Reads:** by-id + by-params (the expected defaults).

- **Reserved read controls served by the listing** (declared = served; undeclared = typed
  400 — a contract, not an omission):

  | Control | Served | Why |
  |---|---|---|
  | pagination (`?first`/`?after`/…) | yes | default |
  | `?orderBy` | yes | default |
  | `?fields` | yes | every Response field and every nested type must then be `*T`/slice + `,omitempty` — a boot guard |
  | `?onlyTotal` | yes | cheap, and operators count tenants |
  | `?includeArchived` | yes | required — archived tenants must stay reachable (§6) |
  | `?search` | **no** | a relational-served view answers free-text search with a typed 400 (`RelationalCapabilityNotification`). Declaring it would promise what the posture cannot serve. Prefix/contains filters over root columns cover the real need. |

- **Computed read fields: none.** `tenant_id` is a stored column, not a computed field —
  deliberately, so it can be filtered and indexed (a computed field can be neither).
- **Field-level read authz: none** (Q4). All fields are visible to any caller that passes
  the permission gate; nothing here is a secret — and per §A.6, `tenant_id` must never be
  treated as one.
- **View backing: relational (`.RelationalSource(repo.Loader)`)** — the project posture on
  record in `scaffold-service/spec.md`, not re-asked. Read-your-writes: a created tenant is
  visible to the very next read, no CDC wait. The view reuses the aggregate's existing
  `repo.Loader`; a second loader on the same table boots fine and is pure waste.
- **Archive regime: kept-but-hidden, revealed by `?includeArchived`.** Not a choice on this
  backing — `DeleteOnArchive()` is a Mongo-projection knob and a relational view composes
  from the source at read time, so there is no document to drop (`shared/read-side.md`).
  Stated rather than defaulted.
- **Filter / sort per field** (low-risk — exact operator tokens per
  `auto-query-handlers.html` at generation time):

  | Field | Filter | Sort |
  |---|---|---|
  | `tenantId` | equality · in-list | no |
  | `name` | equality · in-list · prefix · contains | yes |
  | `workspace` | equality · in-list · prefix | yes |
  | `description` | contains | no |
  | `status` | equality · in-list | no |
  | `createdAt` / `updatedAt` | equality · range (gt/gte/lt/lte) | yes |
  | archived state | via `?includeArchived` | no |

---

## 10. Authorization                          [required — both slots]

- **Permission gate (Layer 1)** (proposed — adopted from the taxonomy `README.md` already
  records, so the deployment is not asked to grant synonyms for one thing). The same strings
  apply to REST routes and to GraphQL fields — authorization is not surface-specific:

  | Operation | Permission |
  |---|---|
  | list, read one | `tenant:read` |
  | create | `tenant:insert` |
  | partial update (including a status transition) | `tenant:update` |
  | archive, unarchive | `tenant:archive` |

  The action spells the operation, and one verb covers a verb and its undo. Permissions are
  compared exactly against the caller's token, so the only wrong answer is one that does not
  match what the deployment grants.

  **Two costs of routing status through the ordinary PATCH, accepted knowingly** (§B Q9;
  the alternative was dedicated `activate` / `suspend` intent routes under their own
  `tenant:status` permission):

  1. **One permission covers registry editing and commercial state.** Whoever may fix a
     typo in a tenant's name may also lift the suspension of a delinquent account. If those
     ever need to be different actors — and in most billing organizations they eventually
     do — the fix is to split the intent routes out, which is additive and breaks nothing
     already generated.
  2. **The audit timeline cannot tell the intentions apart.** A rename and a suspension both
     land as verb `update`; only the field diff distinguishes them. A dedicated route would
     have given each its own `actionName`.

- **Data-access (Layer 2/3): anyone holding the permission sees and edits every row** (Q4).
  No owner check in `BuildRules`, no tenant filter in `ToCriteria` — the command and query
  `ctx` gateways carry no row-scoping predicate.

  Recorded deliberately, not by omission: this is a **platform registry**, and
  `tenant:insert` is already a platform-level operation. The two alternatives (scope a caller
  to their own tenant; hybrid via a reserved platform tenant) both depend on a `tenant_id`
  claim that does not exist yet — `User` and token issuance are not built — so writing
  either today produces a rule that is inert. Tightening later is additive: a predicate in
  `ToCriteria` and an owner check in `BuildRules`, no re-scaffolding.

---

## Open questions

**None.** Q1–Q6 were answered at the model gate and are recorded in §B.

## Deviations recorded at generation time

<filled by the run>
