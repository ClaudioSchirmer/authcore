# Spec: User

- **Status:** APPROVED
- **Approved:** maintainer (Cláudio Schirmer Guedes), 2026-08-25 — every ⚠️ OPEN slot
  answered at the model gate. The full decision table is the first section below; the `§B`
  questions keep the reasoning that was weighed, marked ✅ where they were resolved. **One
  decision was made against a current standard, knowingly and on the record:** Q3 takes
  composition rules (8+ runes, all four character classes) where NIST SP 800-63B-4 states a
  SHALL NOT. That is the maintainer's call, it is what was asked for, and §B Q3 keeps both
  sides so a future reader sees a decision rather than an oversight
- **Language:** English (all artifacts) · Portuguese (chat) — per `../../../CLAUDE.md`
  rule 3 and the maintainer's invocation
- **Generation:** `<pending>` — gate 1d, and **read §D first**: this is the first entity in
  the service whose central field (the password) has **no spelling in the generator's spec
  language** today
- **Pin:** omnicore **`v0.60.0`** · `omnicore-gen` **0.40.0** · dialect postgres · Postgres
  SoR, no Mongo, no broker → relational-served views. Same posture every existing entity
  was built under
- **AMENDED 2026-08-25, at the model gate, before approval** — two maintainer instructions,
  and one of them corrected a hole in this spec rather than adding to it:
  1. **"por que o password não pode sair na auditoria"** — the question exposed an
     incomplete claim. `hidden` governs **response bodies only**; the audit event is a
     different copy of the row and `password_hash` reached it. The fix is
     `RedactedField` / the spec language's `redact:` — see §2's *"The password, end to
     end"*, property 6, rewritten. **This is a correction, not an addition.**
  2. **"se arquivado, vira suspenso"** — now U15, its own rule, mirroring `Tenant`'s
     rule 13 verbatim (§7). `Status` and this rule are therefore **DECIDED** in §B Q5,
     not proposed.
  Re-verified at `omnicore-gen` **0.39.0**: `redact` is a first-class spec key (so §D
  shrinks), and the body-fed runtime gap is **unchanged** (so §D's core stands)
- **AMENDED AGAIN 2026-08-25 — the two password operations were REDESIGNED by the
  maintainer at the gate**, and the change is larger than it looks: one of them becomes the
  service's **first unauthenticated write**. Instruction, verbatim: *"o usuário logado (jwt)
  precisa ser o próprio usuário, ou superAdmin"* on the reset; the other *"precisa ficar
  aberto, já que se o usuário quer trocar de senha, então ele não [tem a senha] para gerar o
  JWT"*; *"a mensagem precisa ser genérica, usuário ou senha inválido"*; and *"esse endpoint
  não pode ser por ID, precisa ser por email"*. Folded into §B Q2, §7 (U7a → **U7g**, a
  guard), §9, §10 and the new **§E**, which exists because an open credential endpoint is a
  security surface of its own and did not fit inside a table row.
  **The two instructions arrived naming the SAME route** (`/password-reset`) for both the
  JWT-gated case and the open case. Resolved at the gate the only way the names carry
  meaning, and **confirmed**: the OPEN one is the CHANGE (it verifies the current password),
  the JWT-gated one is the RESET (it does not)

## ✅ Decisions taken at the model gate (2026-08-25)

| Slot | Answer | What it changed |
|---|---|---|
| **Q0** Storage | **flat `users`** | §1 stands as proposed |
| **Q1** Membership | **two collections on `User`** | `user_groups` + `user_roles`, §3. The reverse question ("who is in Engineering?") waits for Mongo — accepted |
| **Q2** Password at create | **yes** | the transient pair is needed. **The gap that made this expensive was fixed upstream at `omnicore-gen` 0.40.0** — §D |
| **Q3** Password policy | **composition rules, as asked — 8+ runes and ALL FOUR classes** | `vos.Password`. NIST-4 says the opposite; the maintainer's call, made knowingly. §B Q3 keeps the record |
| **Q4** Hashing | **Argon2id, OWASP baseline, NO pepper** | m=19 MiB, t=2, p=1, PHC in `VARCHAR(255)` |
| **Q5** Extra fields | **`status` · `passwordChangedAt` · `mustChangePassword` · `emailVerifiedAt` · `givenName` · `familyName`** | `externalId` stays out. ⚠️ `emailVerifiedAt` has **no writer in this model** — see the note under §2 |
| **Name shape** | **a COMPOSITE `vos.PersonName` over `given_name` + `family_name`, no standalone `name` column, and the full-name method lives INSIDE the value object** *(maintainer: "não precisa do name solto" → "faz um VO composto e coloca o método para pegar o nome lá dentro, aí seguimos os padrões")* | exactly `Permission`'s shape (`PermissionKey` over two columns + the rendered token as a computed read field). `name` is served as a **computed read field**, both parts stay filterable and sortable on their own. §2 |
| **Q6** `email` | **IMMUTABLE** | U2 becomes insert-only; a new immutability rule; §8's patchable set shrinks; the open endpoint's lookup key is now permanently stable |
| Password ops | **redesigned** — open CHANGE by e-mail, identity-gated RESET | §B Q2, §E |
| `TenantID` source | **conditional (U1b)** | superadmin names it; a tenant caller inherits it from the claim. **Declarative since `omnicore-gen` 0.40.0** (`bypassMaySet: true`) |
| Read joins | **all three** | root→`tenants`, child→`groups`, child→`roles` |
| Archive | **forces `suspended`** (U15) | mirrors `Tenant` |

| **Route naming** | **as proposed** — the OPEN one is the CHANGE (`PATCH /users/password`, verifies the current password); the JWT one is the RESET (`PATCH /users/{id}/password-reset`, does not) | §B Q2 |
| **Public route path** | **`PATCH /users/password`**, kept inside the `/users` resource | ⚠️ it **collides** with `PATCH /users/{id}` — `password` matches `:id` — so it MUST be registered first, and that ordering is now a hard acceptance check, not a comment. §E |
| **Reset posture** | **self (`sub` == `:id`) · `*:*` · OR `user:reset-password`** | the sixth verb is BACK. A tenant can run its own helpdesk without whoever edits a name being able to take an account. §10 |
| **Brute force** | **`failedLoginAttempts` + `lockedUntil` taken NOW** | §C-4 is no longer deferred — the open endpoint is the writer it was waiting for. New fields in §2, new rules U7h/U7i in §7, and lockout answers the SAME generic 401. §E |

**No ⚠️ OPEN slot remains.**

The **first node** of the graph the README draws — `User → Group → Role → Permission`,
with `User → Role` as the direct second path — and the last one missing. A user is a
person's account **inside exactly one tenant**: the thing a token is minted for, the thing
`tenant_id` is read off, and the only place in this service that holds a credential.

Structurally it is **`Group` with two collections instead of one, plus a secret**. The
tenant-owned flat root, the id-only child rows with the counterpart read across the foreign
key, the per-child attach/detach pair, the one-way archive and the tenant isolation are all
inherited verbatim from `../group/spec.md` and are not re-argued here. **Everything that is
NOT a copy of `Group` is the password**, and that is what most of this spec is about.

## Sources read before writing this spec

| Source | What it settled |
|---|---|
| `../../../README.md` § *One e-mail, one user* | `email` is unique **across the whole platform**, over ACTIVE rows — `CREATE UNIQUE INDEX users_email_key ON users (email) WHERE deleted_at IS NULL`. A person in two tenants is two users, through two corporate addresses. The escape hatch (a global `Identity` + a per-tenant `User`) is recorded there **as an escape hatch, not as a plan** → §B Q0 |
| `../../../README.md` § *What is scoped to a tenant* · § *Where the platform operators live* | `User` is `tenant_id` **NOT NULL**; platform operators live in a reserved platform tenant rather than behind a nullable owner |
| `../../../README.md` § *The shape we are building towards* | effective permissions = group path ∪ direct path, **no precedence and no deny rule**. Every edge is many-to-many except `tenant_id` |
| `../group/spec.md` (APPROVED) | the pattern this entity follows one level up, and the five decisions it inherits: id-only child storage with the counterpart read across the FK, the ATTACH/DETACH pair, one-way archive, the transitive no-escalation rule + its wildcard interlock, and `<resource>:grant` as a fifth verb |
| `../role/spec.md` · `../permission/spec.md` · `../tenant/spec.md` (all APPROVED) | the local flavor: substance-validated text, archive-never-delete, service pre-check + DB backstop for uniqueness, `TenantID` in the body and never `assignedFrom: identity-claim`, the G0 barrier rule, suspension ≠ archiving |
| `internal/domain/vos/display_name.go` (read, lines 1–12) | **`DisplayName` is for THINGS, not people** — the file says so in its own header, and reserves `PersonName` for when a person arrives. That decision is the maintainer's, already written, and §2 obeys it |
| `internal/domain/vos/text_predicates.go` (read) | the shared anti-junk predicates a new VO composes from: `runeLen`, `distinctRunes`, `hasRunOfIdenticalRunes`, `hasLetter`, `countWords`, `hasVowel`, `isTrimmedAndSingleSpaced` |
| `internal/domain/notifications.go` (read) | which notifications already exist and are already translated — `RoleNotAvailableInTenantNotification`, `CannotGrantRoleWithUnheldPermissionsNotification`, `CannotGrantWildcardRoleNotification` are **reused** by §7 rather than duplicated |
| omnicore `v0.60.0` `/docs` | `token-issuance` (the boundary: the framework mints tokens and **never sees a password**), `value-objects` (the automatic VO pass and its `IgnoreValueObject` escape), `authz-seams` (`Restrict`), `lifecycle-hooks`, `custom-command-handler`, `read-joins`, `relational-view` |
| `omnicore-gen 0.38.0` `explain keys` · `explain coverage` (run) | what the spec language can and cannot say — the gap in §D is quoted from its own output, not inferred |
| NIST **SP 800-63B-4** · OWASP *Password Storage Cheat Sheet* (web, 2026-08-25) | the two external standards §A-8…§A-11 are built on |

### Verified framework facts (read at this pin, not assumed)

| Fact | Evidence at `v0.60.0` |
|---|---|
| **The framework never sees a password and never decides who is authentic.** `Issuer.Issue` mints for a subject the *service* already decided is authentic; "User/credential entity, password hashing, lockout and rate limiting … all of it is domain" | `token-issuance.html`, § *Security* and § *What stays in the consumer service* |
| The automatic value-object pass walks **every exported top-level field of the entity** by reflection — it is NOT scoped to fields the `TableSchema` persists. So a field that exists only in memory still gets its VO rule enforced on every write | `value-objects.html`, § *Validation is automatic (both kinds)* |
| That pass can be switched off **per mode** with `r.IgnoreValueObject("Field")` inside a mode gate | `value-objects.html`, § *Adjusting the automatic pass* |
| A field may be kept out of **every** response body — by-id, each listing row, the write results and the CSV/XLSX exports — with `hidden` | `omnicore-gen explain keys`, `fields[].hidden`; shipped in this project by `Permission.Resource`/`Action` |
| **`hidden` governs the RESPONSE and nothing else.** The framework makes its own copies of a row — the outbox payload (and from it the topic, the consumers, both failure ledgers and the projected document) and the **audit event** — and *"declaring a field persists it — and, by the same declaration, sends it to the outbox payload … and to the audit event"* | `table-schema.html`, § *RedactedField* |
| `RedactedField` keeps the real value in the column and in the hydrated entity while masking it in each of those copies. **Both axes are mandatory** — `core.InSync(r)` and `core.InAudit(r)`; a missing one is a construction panic, because *"silence would have to default either to leaking or to guessing, and neither is its call"* | `table-schema.html`, § *The two axes — both mandatory* |
| The closed redactor family: `core.Plain()` · `core.RedactWith(v)` (fixed) · `core.RedactKeepLast(n)` (string only) · `core.RedactUsing(f)` (a pure function, string only). **There is no `Omit`** — every redactor keeps the key and changes the value, because an absent key already means "the 1:1 sibling row was removed" | `table-schema.html`, § *The closed family* |
| Audit redaction runs **AFTER** the delta is computed, so the trail still records **that** a field changed while masking both sides: `{"field": "…", "from": "***", "to": "***"}`. On an insert/delete snapshot the key is present and masked, never dropped | `audit.html`, § *Redacted fields — what changed without what it changed to* |
| **Nothing on the read side is refused because a field is redacted** — filters, ordering and `?search=` keep working. *"The framework offers the mechanism and does not decide the project's policy"* | `table-schema.html`, § *RedactedField*. This is why §9 keeping `passwordHash` out of every filter is a **separate** decision that still has to be made by hand |
| One successful write emits exactly one `AuditEvent`, written **inside the write's transaction**, and by default routed to BOTH destinations: the `audit_events` row **and** a `slog` echo to the log aggregator | `audit.html`, § *How it works* and § *Two destinations* |
| `redact` is a first-class key of the generator's spec language at **0.39.0** — `fields[].redact.inSync` / `.inAudit`, each with `kind: plain \| fixed \| keep-last \| hook` | `omnicore-gen 0.39.0 explain keys` (run); `explain coverage` lists *"redacted fields"* as generated. **It was NOT in 0.38.0** — this capability landed between the two builds |
| `fields[].runtime` is **still token-fed only at 0.39.0** — the wording is byte-identical to 0.38.0's | `omnicore-gen 0.39.0 explain keys` (run, and diffed against 0.38.0). §D's gap is unchanged |
| A **persisted** field the server fills and no write request carries is `assignedFrom: derived` — "from the entity's own fields, **by a rule you write**" | `omnicore-gen explain vocabulary`, `fields[].assignedFrom` |
| `fields[].runtime` is **token-fed only** — "never persisted, fed from the caller's token (see `claim`), existing only for the rules to read" | `omnicore-gen explain keys`, `fields[].runtime` / `fields[].claim`. **This is the gap §D names**: there is no spelling for a runtime field fed from the request BODY |
| A read join is declared once on the repository and inherited by every consumer of that loader; a join **on a collection** fills its fields on every loaded entry and is **not** addressable in a criteria; its predicate is always `fk = target.id` | `read-joins.html` — re-confirmed unchanged from `../group/spec.md`'s reading at `v0.57.0` |
| A relational view serves the aggregate's 1:N children but refuses filter/sort **on a child field** with a typed 400, never a 500 | `relational-view.html` — likewise unchanged |
| `Identity.HasPermission` **panics** on any argument containing `*`; `Identity.IsSuperAdmin()` is the sanctioned `*:*` question | `application/configuration/identity.go` — read for `../group/spec.md`, unchanged at this pin |
| `TenantMismatchNotification` / `TenantMissingNotification` are framework-owned and already translated in all seven catalogs | established by `../role/spec.md`, unchanged |

---

## §A — What large platforms actually do with a user, a password and a membership

Requested explicitly at the invocation (*"me ajuda a elaborar para fazermos algo parecido
com grandes empresas"*). Each line ends with where it lands here, so nothing is decoration.

| # | What the big platforms / standards do | Where it lands here |
|---|---|---|
| 1 | **The user is the credential holder, and the credential is a sub-object.** Okta splits `profile` (login, email, firstName, lastName) from `credentials` (password, recovery question, provider). SCIM 2.0 puts `password` on the `User` resource but as its own attribute with its own mutability | §2 and §4 — one root table, the credential as columns on it rather than a 1:1 satellite. §4 says why the satellite loses here |
| 2 | **The password is `writeOnly` and `returned: never`.** RFC 7643 §4.1.1 declares exactly that on `User.password`: it goes in, it never comes out — not in the create response, not in a read, not in a listing | §2 — the plaintext is never stored and the hash is `hidden`. This is the invocation's *"no endpoint e consulta, nunca volta a senha"*, and it is also the standard |
| 3 | **A login handle distinct from the e-mail is common.** Okta has `login` beside `email`; Entra has `userPrincipalName` beside `mail`; SCIM requires `userName` | **Not taken, and the README already decided it**: the e-mail *is* the login, globally unique over active rows. §2 |
| 4 | **The display name is one required field; the split parts are optional.** Entra requires `displayName` and leaves `givenName`/`surname` optional; SCIM requires `userName` and makes `name.givenName`/`familyName` optional sub-attributes; Okta is the outlier that requires `firstName` + `lastName` | §2 — one required `name`, with the split offered in §C-3 rather than baked. Mononyms and non-Western name order are why the split is offered and not imposed |
| 5 | **Account state is a first-class enum, separate from deletion.** Okta: `STAGED` · `PROVISIONED` · `ACTIVE` · `RECOVERY` · `LOCKED_OUT` · `PASSWORD_EXPIRED` · `SUSPENDED` · `DEPROVISIONED`. Entra: `accountEnabled` (a bool) plus a soft-deleted state with a 30-day restore window | §2/§5 — a deliberately small `active` \| `suspended`, mirroring `Tenant`'s "suspension is not archiving". The states Okta ships that describe a *flow* (`RECOVERY`, `PASSWORD_EXPIRED`, `STAGED`) are refused as fields until the flow exists to move them |
| 6 | **Membership is edited on the GROUP, and read from both sides.** Okta `PUT /groups/{gid}/users/{uid}`; Entra `POST /groups/{id}/members/$ref`; SCIM makes `User.groups` `mutability: readOnly` and says membership is changed via the Group resource. AWS IAM is the exception — `AddUserToGroup` is a standalone action naming both | **§B Q1 — this is the one place where doing what was asked diverges from what the big platforms do**, and it is a real fork, not a formality |
| 7 | **A direct role grant to a user exists beside the group path, everywhere.** Entra assigns directory roles to users directly; Okta assigns admin roles to users; AWS attaches policies to a user as well as to a group | §3 — `user_roles`, exactly the README's "granted directly" arrow |
| 8 | **Password storage: Argon2id is the current first recommendation.** OWASP's *Password Storage Cheat Sheet* names Argon2id with a baseline of **m = 19 MiB, t = 2, p = 1** (≈100 ms on a modern server core), and a higher-memory alternative of m = 47 MiB, t = 1, p = 1. bcrypt remains acceptable with a work factor of 10+ | **§B Q4** — proposed Argon2id at the OWASP baseline, stored as a self-describing PHC string so the parameters can be raised later without a migration |
| 9 | **Password length: NIST SP 800-63B-4 requires a minimum of 15 characters when the password is the ONLY factor**, and permits 8 only when it is part of multi-factor authentication. Maximum must be at least 64 | **§B Q3** — and it bites, because this service has no second factor today, so the password *is* single-factor by definition |
| 10 | **NIST-4 forbids composition rules**: *"Other composition requirements for passwords SHALL NOT be imposed"* — no mandatory upper/lower/digit/symbol classes, no periodic rotation, no password hints | **§B Q3 — and this is the one place the invocation and the standard disagree.** The invocation asked for *"requisitos mínimos de aceite, caracteres especiais e etc."*; NIST-4 says do not. §B Q3 lays out both and names the standard that DOES require classes (PCI DSS v4.0 §8.3.6: 12 characters, alphabetic **and** numeric) |
| 11 | **NIST-4 requires a blocklist check** against known-common and known-breached passwords, comparing the **entire** password, not substrings | §B Q3 option C, and §C-5. Deferred by default: it needs a corpus this repository does not have, and shipping a 20-entry toy list is worse than shipping none, because it reads as compliance |
| 12 | **A typed-twice confirmation lives at the API/UI boundary, not in the stored model.** Nobody stores it; Okta and Entra do not model it at all — it is the client's job in their APIs, and a form field in their consoles | §2/§7 — taken as the invocation asks, as a **transient field with a domain rule**, so the mismatch answers in the same 422 envelope and the same seven languages as every other validation failure, instead of being a special case at the edge |
| 13 | **Changing a password is its own operation, never a field on the update.** Okta `POST /users/{id}/credentials/change_password` takes `oldPassword` + `newPassword`; Entra `POST /me/changePassword` takes `currentPassword` + `newPassword`; the admin-side reset is a *different* call with a *different* permission | §B Q2 and §9/§10 — a dedicated operation, and a second one for the admin reset that does not know the current password |
| 14 | **Lockout, failed-attempt counters and last-login are login-flow state.** Every platform has them; all of them are written by the authentication path | **Deferred, recorded** — §C-4. This service has no login route yet (`Issuer` exists in the framework; `POST /auth/login` does not exist here), so those columns would be written by nobody |

**Where this design deliberately deviates:** the big platforms let an administrator attach a
group or a role to a user with one coarse permission and bolt escalation protection on
afterwards. This service does not — `Group` already decided that the domain guarantees no
escalation *and* the deployment can delegate the edge separately (`group:grant`). §7 and §10
carry that decision one level up, where it matters most: attaching a **group** to a user is
a **three-hop** grant (group → roles → permissions), the longest reach in the whole model.

---

## §B — ✅ The questions that were asked at the gate (all ANSWERED; kept for the reasoning)

*Every question below is ANSWERED — the verdict is the `✅` block inside each one, and the
decision table at the top of this file is the index. The option tables are kept as the
record of what was weighed, not as anything still to decide.*

### ✅ Q0 — Flat `users` table, or the README's escape hatch (`Identity` + role)?

**Why this is asked even though the README appears to answer it.** The identity smell is at
its strongest here: `name` + `email` is a person's identity, and `password_hash`,
`tenant_id`, groups and roles are what that person's *membership* carries. That is the
textbook party-role shape, and the README already writes the alternative down:

> if self-employed customers ever become normal here, the answer is not to loosen this index
> but to **split the credential from the membership** — a global `Identity` holding e-mail,
> password and MFA, with `User` demoted to the per-tenant link. Recorded as the escape
> hatch, not as a plan.

| | **A — flat `users`** (proposed) | **B — SharedBase now: `identities` + the `users` role** |
|---|---|---|
| The model | one table; a person in two tenants is two rows with two addresses | `identities` (email, password_hash, MFA later) + `users` (tenant_id, name, groups, roles) as a role over it |
| `email` unique | globally, active-only — exactly the README's index | on the identity, natively |
| One person, two tenants | **impossible with one address** — the README states this cost and accepts it | native, one credential |
| Natural key (the UUIDv5 seed) | N/A | would be `email` — and an e-mail is **re-assignable** (the README says archiving releases it), which is a poor natural key: a recycled address would resolve to the retired person's identity id |
| Cost of changing later | a real migration: new table, PK re-derivation, data move, every FK re-pointed | none — it is the shape |
| Cost of choosing it now | none | one more aggregate, upsert semantics on every create, and an identity view that **this posture cannot serve** (`SharedBaseView` needs Mongo; the generator also refuses it at 0.38.0) |

**Recommendation: A, and the natural-key line is why** — B's dedup key would have to be the
e-mail, and this service deliberately lets an archived user's address be reissued. A shared
identity keyed on a recyclable value merges two different people. The README's escape hatch
stays exactly that; if it is ever taken, the credential moves to a *new* aggregate with a
non-recyclable key, which is a different design from "make `users` a role today".

> ### ✅ ANSWERED: **A — flat `users`.** The table above is the record of what was weighed.

### ✅ Q1 — Where does membership live: two collections on `User`, or its own aggregate?

The invocation asked for **"grupos e roles lista"** on the user. §A-6 says the big platforms
edit group membership on the **group**. And `../group/spec.md` says, in its own words:
*"Membership is out of scope, by design. `User ↔ Group` is its own aggregate, later."* Three
coherent shapes, and the choice is a regenerate-everything one.

| | **A — `user_groups` + `user_roles` as collections on `User`** (proposed) | **B — membership on `Group` (`group_members`)** | **C — `UserGroup` / `UserRole` as their own root aggregates** |
|---|---|---|---|
| What was asked for | **exactly this** | no — the user carries no lists | no — the user carries no lists |
| `GET /users/{id}` shows the person's groups and roles | **yes**, with each entry's key and name read across the FK | no | no |
| "Who is in Engineering?" | **not answerable** — a child field is not addressable in a criteria on a relational view | yes, from `GET /groups/{id}` | **yes** — the edge's own columns are root columns, so `?filter[groupId][eq]=…` works |
| "Which groups is Maria in?" | yes | not answerable | yes |
| Matches the token-issuance path (user → groups → roles → permissions) | **yes, and this is the critical path** — one read of the user hands back every group id and role id | no — the issuer would have to scan groups | yes, with one extra listing read |
| Consistency with what is already built | **exact** — it is `Group`'s `group_roles`, twice | changes a BUILT entity (`/omnicore:evolve-entity` on `Group`) | introduces a shape the service does not have |
| Industry analogue | AWS IAM (`AddUserToGroup` names both sides) | Okta / Entra / SCIM | SCIM's `Group.members` read the other way; also how most RBAC schemas are drawn |
| Cost | the reverse question waits for Mongo, exactly as `Group`'s and `Role`'s already do | re-opening an approved, generated entity | two more aggregates, two more specs, two more permission families |

**Recommendation: A.** Two reasons, in order. First, the **forward** walk is the one the
token issuer runs on every login, and A serves it in a single read; the reverse direction is
an access-review question, and this service has already accepted twice — in `../role/spec.md`
§9 and `../group/spec.md` §C-7 — that the reverse direction waits for
`/omnicore:configure` adding Mongo. Taking A keeps that one accepted gap one gap, instead of
opening a second, differently-shaped one. Second, it is what was asked for.

**What A costs, stated plainly and not softened:** an admin console listing *"the members of
this group"* cannot be built from this API. That is the single most common screen in an
identity product, and it will be missing until Mongo arrives or a `UserGroup` aggregate is
added later. C is the only option that answers both directions on the current posture.

> ### ✅ ANSWERED: **A — two collections on `User`.** The reverse question ("who is in
> Engineering?") is an accepted gap, consistent with `Role` and `Group`, and closes the day
> `/omnicore:configure` adds Mongo.

### ✅ Q2 — Does the password arrive at create? *(the operations half is now DECIDED — below)*

The password cannot be a patchable field: a `PATCH /users/{id}` carrying `password` would
let anyone holding `user:update` overwrite a credential, would put a secret in the same
audit row as a rename, and could not ask for the current one. So the password needs its own
operations — the question is which, and whether `POST /users` carries one at all.

| | **A — password at create + change + admin reset** (proposed) | **B — no password at create; set-on-invite + change + admin reset** |
|---|---|---|
| `POST /users` body | `password` + `passwordConfirmation`, required | no credential at all; `password_hash` starts NULL |
| First sign-in | the admin communicates a temporary password; `mustChangePassword` forces a change | an activation/invite flow — **which does not exist in this service and is not in this spec** |
| Matches the invocation | **yes** (*"modelar o usuário, com … senha"*, *"o endpoint receba uma confirmação de senha"*) | no |
| Generator impact | needs the transient body-fed pair → **§D's gap** | the generated tree stays clean; the whole credential lives in hand-written commands |
| Industry analogue | Entra's `passwordProfile` on create; Okta's create-with-credentials | Okta's `STAGED` user + activation e-mail; AWS IAM (a user has no console password until a `LoginProfile` is created) |
| The admin knows the password | **yes, momentarily** — a real, named weakness of A, mitigated by `mustChangePassword` | no |

### ✅ The two operations — DECIDED at the gate (2026-08-25), and redesigned

**The maintainer's instruction replaced what this spec first proposed.** The original draft
put both operations behind a permission and both under `/users/{id}/…`. Both halves were
wrong, and the second one is wrong *mechanically*, not just by taste — see §E.

| | **CHANGE — open** | **RESET — authenticated** |
|---|---|---|
| Route | `PATCH /users/password` *(the routing collision it carries is handled in §E)* | `PATCH /users/{id}/password-reset` |
| Authentication | **none — `auth.publicRoutes`** | JWT required |
| Authorization | none. Possession of the current password **is** the authorization | **self (`sub` == `:id`) or `*:*` superadmin** — no permission verb |
| Identified by | **`email` in the body** — never a path id | the path id |
| Body | `email`, `currentPassword`, `password`, `passwordConfirmation` | `password`, `passwordConfirmation` |
| Verifies the current password | **yes** | no — that is what makes it a *reset* |
| Failure answer | **one generic message for every credential failure** — *"usuário ou senha inválido"* | ordinary 403 / 404 |
| `mustChangePassword` | **cleared** | **set** |

**Why the open one exists, in the maintainer's own reasoning:** a user who needs to change
their password may be exactly the user who **cannot obtain a JWT** — `mustChangePassword` is
set, the credential is expired, or they are simply at a login screen that refused them.
Gating the change on a token they cannot get is a deadlock, and it is why every platform's
change-password flow accepts the current password as the credential instead of a session.

**Why it cannot be by id — and this is not a preference.** `auth.publicRoutes` is an
**exact `METHOD /path` match with no globs** (verified in `yaml-reference.html`), so a route
carrying a path parameter *cannot be declared public at all*. `PATCH /users/{id}/password`
is unreachable as an open route by construction. Beyond the mechanism: an unauthenticated
caller does not know their own UUID — they know the address they type into a login box, and
the README's **global** unique index on `email` is what makes that address resolve to
exactly one user platform-wide, with no tenant claim to scope it by.

**✅ CONFIRMED at the gate — the two instructions had named the same route.** They arrived as
*"no endpoint `PATCH /users/{id}/password-reset`, o usuário logado (jwt) precisa ser o
próprio usuário, ou superAdmin"* **and** *"o `PATCH /users/{id}/password-reset` precisa ficar
aberto"*. Those cannot both hold. The table above resolves it the only way the two names can
carry meaning — **the endpoint that verifies the current password is the CHANGE and is the
open one; the endpoint that does not verify it is the RESET and is the gated one** — because
a "reset" that demands the current password is not a reset, and because the generic
*"usuário ou senha inválido"* message only has something to be generic **about** on the
endpoint that checks credentials. **Confirmed as written at the gate**; swapping them
later would be a rename and nothing else moves.

**✅ A second thing, also decided — the reset takes a THIRD acceptor.** As first instructed,
"self or superadmin" would have meant a tenant admin cannot reset their own user's password. Only the person themselves and a platform `*:*` operator
can. That is a defensible, tight posture, and it is also a helpdesk that has to escalate to
the platform for every forgotten password. Three ways to read it:

- **as instructed** — self or `*:*` only *(what the spec now says)*;
- **plus a permission** — self, `*:*`, **or** a holder of `user:reset-password`, which is
  what the first draft proposed and what Entra and Okta both do (a delegated
  password-administrator role);
- self only, with the superadmin reaching it through impersonation — **not recommended**,
  the service has no impersonation and inventing one here would be worse.

> ### ✅ ANSWERED: **the second — self, `*:*`, OR `user:reset-password`.** The sixth verb is
> declared (§10). It is an ADDITIONAL acceptor, never a replacement: self-service stays
> independent of any grant, so the operation cannot be made unreachable by forgetting to
> issue one.

**Q2's remaining open half:** whether the password arrives **at create** (A) or the user
starts credential-less (B). The two operations above are settled either way — under B the
open CHANGE endpoint is simply the only way a password is ever set, which needs an invite
token this service does not have.

> ### ✅ ANSWERED, all three parts:
> - **create carries the password** (A);
> - **the route names stand as written** — the OPEN one is the CHANGE, the JWT one is the RESET;
> - **the reset accepts self OR `*:*` OR `user:reset-password`** — the sixth verb is IN, as an
>   additional acceptor, so a tenant can run its own helpdesk while self-service never depends
>   on a grant.

### ✅ Q3 — The password policy: NIST-4, or composition rules as asked?

The invocation asked for *"requisitos mínimos de aceite, caracteres especiais e etc."*
**NIST SP 800-63B-4 says the opposite, in a SHALL NOT**: *"Other composition requirements
for passwords shall not be imposed."* It also sets the floor at **15 characters when the
password is the only factor** — which is this service today — and 8 only inside MFA. This
is the one place where doing exactly what was asked means deliberately not following the
current standard, so it is put in front of you rather than decided quietly.

| | **A — NIST-4 clean** (proposed) | **B — composition rules, as asked** | **C — PCI-DSS-shaped middle** |
|---|---|---|---|
| Minimum length | **15** (single-factor floor) | 8–12, because classes are doing the work | **12** |
| Maximum length | 128 (≥64 required; 128 caps the Argon2 input) | same | same |
| Character classes | **none required** — all Unicode accepted, spaces included | at least 3 of 4: lower, upper, digit, symbol | alphabetic **and** numeric (PCI DSS v4.0 §8.3.6) |
| Blocklist | recommended, deferred → §C-5 | same | same |
| Context check (password must not contain the e-mail local part, or any ≥4-rune word of the name) | **yes** | yes | yes |
| Standard behind it | NIST SP 800-63B-4 | none current — it is what most enterprises still deploy | PCI DSS v4.0, for cardholder environments |
| What it costs | users who like `P@ss1234` are refused for being 8 characters, not for lacking a symbol; long passphrases pass | measurably weaker in practice (classes push users to `Password1!`) and NIST-non-compliant | a compromise that satisfies auditors of one framework and neither of the others |

> ### ✅ ANSWERED: **B — composition rules, as asked: 8–128 runes and ALL FOUR classes**
> (lowercase, uppercase, digit, symbol), Unicode-aware, plus the context rule (U6b).
> **Decided knowingly against NIST SP 800-63B-4**, which states a SHALL NOT on composition
> requirements and sets 15 as the single-factor floor. The record below is kept precisely so
> this reads as a decision. It is one value object; revisiting it later changes ~15 lines and
> no structure — and MFA (§C-8) is what would make the NIST reading cheap to adopt.

~~**Recommendation: A**~~ — *superseded by the answer above; kept as the reasoning that was
weighed.* It was the only option of the three that
matches a current standard, given this service has no second factor. If a specific
compliance regime is on the roadmap (PCI, an ISO 27001 auditor with a checklist, a
customer's security questionnaire) that changes the answer, and **B is a legitimate choice
made for a legitimate reason** — say so and it goes in exactly as asked. What must not
happen is B chosen by default.

**Everything in A, B and C is enforced by the same value object** (`vos.Password`), so this
answer changes ~15 lines and no structure. It is asked because it is a policy, not because
it is expensive.

*(The follow-up — which classes, and how many — was asked and answered: **all four**.)*

### ✅ Q4 — Argon2id or bcrypt, and is there a pepper?

| | **Argon2id** (proposed) | **bcrypt** |
|---|---|---|
| OWASP status | **first recommendation** | acceptable, "legacy but fine", work factor ≥ 10 |
| Parameters | m = 19 MiB, t = 2, p = 1 (the OWASP baseline, ≈100 ms/core) | cost 12 |
| Go package | `golang.org/x/crypto/argon2` | `golang.org/x/crypto/bcrypt` |
| New dependency | **no new module** — `golang.org/x/crypto` is already in `go.sum` as an indirect; it becomes a direct require | identical |
| Password length cap | none (128 is our own cap) | **72 bytes, silently truncated** — a real footgun that has to be guarded explicitly |
| Stored form | PHC string `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>` — self-describing, so raising the parameters later needs **no migration**, only a rehash-on-next-login | `$2a$12$…`, also self-describing |
| Memory cost per concurrent login | **19 MiB** — 100 simultaneous logins is ~2 GB. Named because it is a capacity question, not a security one | negligible |

**Recommendation: Argon2id at the OWASP baseline**, PHC-encoded into `VARCHAR(255)`.

**Pepper (a server-side secret HMAC'd into the password before hashing): recommended NO**,
and offered in §C-6. It defends only the "database stolen, application secrets not stolen"
case, and it introduces a key whose loss bricks every password in the system with no
recovery. If it is wanted it belongs with the signing-key material (`${ENV_VAR}`), and it
should be decided now rather than later — adding one later means every stored hash is
un-verifiable until each user next signs in.

> ### ✅ ANSWERED: **Argon2id at the OWASP baseline, NO pepper.**

### ✅ Q5 — Which fields beyond the four that were asked for?

`name`, `email`, `password`, `tenantID`, groups and roles are in. Everything below is
enterprise furniture: each is cheap now, each is a migration later, and none is free.

| Field | What it buys | Who writes it | Recommendation |
|---|---|---|---|
| `status` — `active` \| `suspended` | reversible deactivation that is **not** archiving. Exactly `Tenant`'s distinction: a suspended user stays listed, keeps their memberships and stops signing in | `PATCH /users/{id}` + **U15 on archive** | ✅ **DECIDED at the gate** — taken, together with *"se arquivado, vira suspenso"* (U15) |
| `passwordChangedAt` | "whose password predates the incident" — the only report that matters after a breach | the two password operations | **take** |
| `mustChangePassword` | forces a rotation after an admin-set or admin-reset password. Entra's `forceChangePasswordNextSignIn` | create + reset (set), change (clear) | **take** — it is what makes Q2-A tolerable |
| `emailVerifiedAt` | proof the address is reachable | **nobody** — there is no verification flow | **skip** → §C-2 |
| `lastLoginAt`, `failedLoginAttempts`, `lockedUntil` | lockout and dormancy reports | **nobody** — there is no login route in this service yet | **skip** → §C-4 |
| `externalId` | SCIM/IdP provisioning handle, exactly as offered on `Group` | a provisioning client | **skip for now** → §C-1, same call `Group` made |
| `locale`, `timezone` | per-user rendering; this service already ships seven catalogs | the user | **skip** → §C-7 |
| `givenName` / `familyName` beside `name` | sortable-by-surname listings, salutations | the caller | **skip** → §C-3 |

> ### ✅ ANSWERED: **`status` · `passwordChangedAt` · `mustChangePassword` · `emailVerifiedAt`
> · the name split** — and, from §E, **`failedLoginAttempts` + `lockedUntil`**. Only
> `externalId` and `locale`/`timezone` stay out.
>
> **Two of those were taken against the recommendation, and both are recorded honestly rather
> than quietly reclassified:**
> - **`emailVerifiedAt` has no writer in this model** — §2 carries the note. What it buys is
>   the migration; what it costs is a column that reads `null` for everyone until §C-2 exists.
> - **the name split replaced `name` entirely** and then became a COMPOSITE value object with
>   the full-name method inside it — which turned out better than either the original single
>   field or a bare split, because it lands on `Permission`'s existing shape.



### ✅ Q6 — Is `email` patchable?

It is the login handle, the global unique key and, once the invite flow exists, the delivery
address.

| | **A — patchable** (proposed) | **B — immutable, like `workspace` / `key`** |
|---|---|---|
| Real case it serves | a legal name change; a company migrating `maria@old.com` → `maria@new.com` | none — the answer is "archive and re-create", which loses every membership |
| Risk | changing it changes who can sign in; an attacker with `user:update` moves the account to their own address | none |
| Mitigation | it is already behind `user:update` **and** tenant isolation, and the change is audited like every other write | — |
| Precedent here | `Tenant.name` is patchable; `Tenant.workspace` and `Role.key`/`Group.key` are not | the handles are frozen because **external systems reference them**; an e-mail is referenced by a human inbox, not by a foreign key |

> ### ✅ ANSWERED: **B — `email` is IMMUTABLE** (U2b).
>
> Three consequences, all folded in: **U2 becomes `IfInsert` only** (no update to
> exclude-self against); **§8's patchable set shrinks to `givenName`, `familyName`, `status`**;
> and the open endpoint's lookup key becomes permanently stable, which is a real gain for a
> credential route. **The cost, named:** fixing a typo in an address means archiving and
> re-creating, which loses every group and role the user had. §C-2 also gets easier — a
> verified address can never silently become unverified by an edit.

~~**Recommendation: A**~~ — *superseded; kept as the reasoning that was weighed.*

---

## 1. Storage model                                    [high-risk — confirm]

- **Kind: flat** — ✅ **DECIDED (§B Q0).** The identity smell is present and was put in front
  of the maintainer rather than self-answered; flat won on the natural-key argument (a
  SharedBase here would have to key on the e-mail, and this service deliberately lets an
  archived user's address be reissued).

- **ER sketch**:

```
tenants                users                                user_groups
───────                ─────                                ───────────
id       UUID PK  ┌──── id                UUID PK      ┌──── id         UUID PK
workspace  …   ───┘     tenant_id         UUID FK ─────┘     user_id    UUID FK → users.id
status     …          ┌ given_name        VARCHAR(75)        group_id   UUID FK → groups.id
              PersonName                                     created_at / updated_at
              (composite) └ family_name    VARCHAR(75)        deleted_at
                        email             VARCHAR(254)                │
                        email_verified_at TIMESTAMPTZ NULL         groups
                        password_hash     VARCHAR(255)                │
                        password_changed_at TIMESTAMPTZ NULL          │
                        must_change_password BOOLEAN                   │
                        status            VARCHAR(16)                 │
                        revision / created_at                         │
                        updated_at / deleted_at              user_roles
                                                             ──────────
                                                       ┌──── id         UUID PK
                                                       │     user_id    UUID FK → users.id
                                                       └───  role_id    UUID FK → roles.id
                                                             created_at / updated_at
                                                             deleted_at
                                                                    │
                                                                 roles

UNIQUE (email)             WHERE deleted_at IS NULL   -- users, GLOBAL, per the README
UNIQUE (user_id, group_id) WHERE deleted_at IS NULL   -- user_groups
UNIQUE (user_id, role_id)  WHERE deleted_at IS NULL   -- user_roles
INDEX  (tenant_id)                                    -- users, the isolation filter
INDEX  (user_id)                                      -- both children, the parent read
INDEX  (group_id) · INDEX (role_id)                   -- the reverse walk, when it exists
```

| Table | Description (becomes the table COMMENT) |
|---|---|
| `users` | A person's account inside exactly one tenant: who they are, the credential they sign in with, and the state of that account. The e-mail is unique across the whole platform over active rows — one address is one user, never two. |
| `user_groups` | The groups this user belongs to. One row per membership, holding nothing but the group's id, so a retired-and-recreated group is never silently re-joined. |
| `user_roles` | The roles granted to this user directly, beside anything their groups confer. One row per grant, holding nothing but the role's id. |

- **`users.email` carries a GLOBAL partial unique index**, not a per-tenant one. This is the
  README's decision, quoted in *Sources* above, and it is the only uniqueness in this
  service that is not scoped by `tenant_id`. `ON CONFLICT` is not involved: the index is the
  race backstop behind a domain pre-check (U2), exactly as everywhere else here.
- **`users.tenant_id` FKs to `tenants.id`, `NO ACTION`** — verbatim mirror of `roles` and
  `groups`, for the reason written into `0003_role_manual.up.sql`: a tenant is archived and
  never purged.
- **`user_groups.group_id` → `groups.id`** and **`user_roles.role_id` → `roles.id`**, both
  to the primary key and not to any partial unique index (Postgres will not point a foreign
  key at one). Each buys the EXISTENCE half of U8/U9 for free; the domain rules still earn
  their keep on the ACTIVE and SAME-TENANT halves.
- **`password_hash` is `VARCHAR(255)`, `NOT NULL`** under Q2-A (nullable under Q2-B). 255 is
  sized for a PHC string with a 16-byte salt and a 32-byte hash and leaves room for raised
  parameters; a bcrypt string is 60.
- **No column ever holds a plaintext password**, on any table, at any moment. §2 and §D say
  how the plaintext reaches the rules without one.
- If sharedbase-role: `N/A — flat, pending Q0`.

## 2. Fields

**Root — persisted**

| Field | Go type | VO? | Nullable | Unique | Lives on | `example:` | Description |
|---|---|---|---|---|---|---|---|
| `TenantID` | `domain.ID` | plain (an id) | no | no | root | `0198f3c2-6b41-7c9e-9f2a-6d3b1e77a410` | The tenant this user belongs to. **Conditional source (U1b): a `*:*` superadmin names it in the body; a tenant caller INHERITS it from their own `tenant_id` claim.** Immutable after creation — a user never moves between tenants |
| `Name` | `vos.PersonName` | **new-COMPOSITE** `vos.PersonName` | no | no | root (**2 columns**) | — | The person's name. **ONE value across two columns** — see the parts below |
| `Email` | `vos.Email` | **new-raw** `vos.Email` | no | **yes, GLOBALLY, active-only** | root | `maria@acme.com` | The address this person signs in with. Unique across the whole platform; **immutable after creation** (U2b); archiving the user releases it |
| `EmailVerifiedAt` | `*time.Time` | plain | **yes** | no | root | `2026-08-25T14:03:11Z` | When the address was proven reachable. ⚠️ **Nothing in this model writes it** — see the note below |
| `PasswordHash` | `string` (255) | plain | no *(nullable under Q2-B)* | no | root | *(never shown — see below)* | The irreversible hash of the password, PHC-encoded so its parameters travel with it. `assignedFrom: derived` + `hidden` + **`redact` on both axes** — absent from every write request, from every response body, from the audit event and from the sync payload |
| `PasswordChangedAt` | `*time.Time` | plain | **yes** | no | root | `2026-08-25T14:03:11Z` | When the credential was last set. NULL only for a user who has never had one |
| `MustChangePassword` | `bool` | plain | no | no | root | `true` | Whether the next sign-in must rotate the password. Set by create and by the reset, cleared by the open change |
| `FailedLoginAttempts` | `int` | plain | no | no | root | `0` | Consecutive credential failures on the open endpoint. Incremented by U7h, zeroed on success. **Never on the wire** — `hidden` |
| `LockedUntil` | `*time.Time` | plain | **yes** | no | root | `2026-08-25T14:18:00Z` | When the lockout expires; NULL means not locked. Set by U7h, honoured by U7i. **Never on the wire** — `hidden` |
| `Status` | `vos.UserStatus` | **new-enum** `vos.UserStatus` | no | no | root | `active` | `active` \| `suspended`. Suspension blocks sign-in and keeps the row listed; it is **not** archiving |

Parts of the composite `Name` — written as a nested list, never as two `§2` rows, because
two rows are exactly the flattening this kind exists to replace and a reviewer could not tell
from them that the fields are one value:

| Part | Column | Exposed as | Type | Nullable | `example:` | Filter / sort |
|---|---|---|---|---|---|---|
| `Given` | `given_name` | `givenName` | `string(75)` | no | `Maria` | `eq,in,startswith,istartswith,contains,icontains` · sortable |
| `Family` | `family_name` | `familyName` | `string(75)` | no | `Souza Lima` | same · **sortable, and this is the sort an operator actually wants** |

The composite **as a whole is required**; neither part is nullable.

### Why the name is a COMPOSITE — and why it is the same shape as `Permission`

*(Maintainer at the gate: **"se name vai ser computed, então faz o givenName e familyName um
VO composto e coloca o método para pegar o nome lá dentro, aí seguimos os padrões."**
DECIDED — and it lands this entity on a pattern the project already ships.)*

`Permission` is the precedent, and the parallel is exact:

| | `Permission` | `User` |
|---|---|---|
| The composite | `vos.PermissionKey` | `vos.PersonName` |
| Columns | `resource_name` + `action_name` | `given_name` + `family_name` |
| Rendered on read | `permission` = `resource:action` | `name` = `Given` + `" "` + `Family` |
| Where the rendering lives | a method **on the value object** | a method **on the value object** |
| Parts on the wire | **hidden** — the caller gets the token only | **exposed** — both are filterable and sortable in their own right |

**The test the framework applies, and this passes it:** several columns are one value object —
rather than two fields — when neither part means the thing alone. `Maria` alone is not a
person's name and `Souza Lima` alone is not either; together they are one. That is the same
sentence `../permission/spec.md` writes about `read` and `tenant`.

**The joining method belongs INSIDE the value object**, and that is the whole point of the
instruction: the rule that a full name is *given, a space, family* is a fact about names, not
about `User`. Put it in the aggregate and the next entity that carries a person copies it;
put it on the VO and there is one definition, one place to change it the day the project
needs a locale-aware order (`familyName givenName` for ja-JP is a real requirement, and it is
a change to one method rather than to every read).

**Four boot-traps this shape carries, all named in the pinned docs:**

1. **A composite must NOT declare `Value()`.** `Value()` is what tells the framework a value
   object occupies ONE column; declaring it on a composite is how a struct gets passed to
   `Field(...)` and panics. `vos.PersonName` declares `IsValid` and the full-name method,
   and no `Value()`.
2. **It never reaches `Field(...)`.** It is declared with `Composite(...)` in the
   `TableSchema`, mapping `Given → given_name` and `Family → family_name`.
3. **Only ONE field of this type on the entity.** Composite resolution is **by type**, so two
   `vos.PersonName` fields would be ambiguous and panic. There is exactly one.
4. **Not split across the root and a sibling** (the *once* rule), and no `json:"-"` on a part
   and no custom `json.Marshaler` on the type — both poison the `Old()` ghost, which is a
   JSON round-trip. §4 has no sibling, so the first one cannot happen here.

**`name` stays a computed READ field**, derived from the two stored parts — it is what the
value object's method produces, surfaced once in `FromQueryResult` so REST, GraphQL and any
future export all render it identically. Its limits are unchanged and are in §9:
`?orderBy=name` is a typed 400 and a filter over it is impossible, which is precisely why
both parts are filterable on their own.

> ⚠️ **`EmailVerifiedAt` is taken by decision and has NO WRITER in this model.** Recorded as
> an accepted deviation rather than sold as a feature: there is no verification flow and no
> outbound mail path in this service, so the column is created and stays NULL for every row
> until §C-2 is built. It is not on any write body (server-written by definition), so nothing
> can set it by accident either. What taking it now buys is the migration: adding a nullable
> timestamp later is cheap but is still a migration, and the column being there means the
> verification flow, when it arrives, is code only. **What it costs is a reader seeing
> `emailVerifiedAt: null` on every user and concluding nobody is verified — which is true,
> and is exactly why it is filterable (`eq`), so "verified vs not" is answerable the day it
> starts being written.**
>
> Note also that **U2b freezing the e-mail removes the hardest half of verification**: with
> an immutable address, a verified user can never silently become unverified by an edit, and
> the "a change re-opens verification" problem §C-2 warned about does not exist.

**Root — never persisted, and never in a response**

| Field | Go type | Fed from | Description |
|---|---|---|---|
| `Password` | `vos.Password` | the **request body**, on create and on both password operations | The plaintext. Validated by its value object, hashed into `PasswordHash`, then out of scope. No column, no response, no log, no audit value |
| `PasswordConfirmation` | `string` | the request body, same three operations | The second typing. Compared by U5 and discarded; it is never hashed and never stored |
| `CurrentPassword` | `string` | the request body, on the self-service change only | Verified against `PasswordHash` by U7a |
| `RequestingTenantID` | `domain.ID` | `ctx.Identity().TenantID()` | Layer-2 isolation input, as on `Role` and `Group` |
| `RequestingPrincipalIsSuperAdmin` | `bool` | `ctx.Identity().IsSuperAdmin()` | The `*:*` bypass, never `HasPermission("*:*")` (it panics) |

**Root — read across the foreign key into `tenants`** (mirrors `Role` and `Group` in full)

| Field | Go type | Source | Description |
|---|---|---|---|
| `TenantWorkspace` | `string` | read join → `tenants.workspace` | The owning tenant's handle. Filterable and sortable — a root join's fields are addressable in a criteria |
| `TenantStatus` | `string` | read join → `tenants.status` | The owning tenant's commercial state. **Plain `string`, never the `TenantStatus` enum** — a join field carries no domain type |

**Child `UserGroup`** (`internal/domain/aggregatevos/user_group.go`)

| Field | Go type | Source | Nullable | Unique | `example:` | Description |
|---|---|---|---|---|---|---|
| `GroupID` | `domain.ID` | **stored** `group_id` | no | yes, within the user | `0198f3e0-9c25-7a1f-b73d-5e08c4a29f61` | The group this membership joins — the id, not the key, so a retired-and-recreated group needs an explicit re-join |
| `GroupKey` | `string` | **read join** → `groups.group_key` | no | — | `engineering` | The group's stable handle. Read-only, filled on load |
| `GroupName` | `string` | **read join** → `groups.name` | no | — | `Engineering` | The group's display name. Same contract |

**Child `UserRole`** (`internal/domain/aggregatevos/user_role.go`)

| Field | Go type | Source | Nullable | Unique | `example:` | Description |
|---|---|---|---|---|---|---|
| `RoleID` | `domain.ID` | **stored** `role_id` | no | yes, within the user | `0198f3e0-1a44-7bb2-9c31-77c0d5e1b904` | The role granted directly to this user |
| `RoleKey` | `string` | **read join** → `roles.role_key` | no | — | `billing-manager` | The role's stable handle. Read-only, filled on load |
| `RoleName` | `string` | **read join** → `roles.name` | no | — | `Billing Manager` | The role's display name. Same contract |

### The three read joins — ✅ confirmed at the gate

*(Maintainer, 2026-08-25: **"readJoins para buscar os dados extras das 3 conexões também,
seguir as outras entidades."** DECIDED — all three taken, mirroring `Role` and `Group`.)*

This entity is the first with **three** traversals, one per foreign key it holds, and they
are not all the same kind:

```
declared on UserRepository, with WithJoins, beside WithSchema
────────────────────────────────────────────────────────────
ROOT join          users.tenant_id      = tenants.id
                     → TenantWorkspace  ← tenants.workspace     filterable · sortable
                     → TenantStatus     ← tenants.status        filterable · sortable

CHILD join         user_groups.group_id = groups.id
                     → GroupKey         ← groups.group_key      load-only
                     → GroupName        ← groups.name           load-only

CHILD join         user_roles.role_id   = roles.id
                     → RoleKey          ← roles.role_key        load-only
                     → RoleName         ← roles.name            load-only
```

Five properties, all of them the framework's rather than this model's, and all five verified
at the pin (`read-joins.html`, re-confirmed for this spec):

1. **Declared ONCE on the repository, inherited by every consumer of that loader** — the
   relational view of §9, `repo.FindByID` (the load every write-side auto handler goes
   through), `repo.ScopedReader(ctx)` and any service reading through `repo.Loader`. The
   view declares nothing.
2. **`inner`, and only because all three keys are `NOT NULL`.** Over a nullable foreign key
   an inner join silently drops rows — from `FindByID` too, turning a legitimate write into
   a 404 — and inside a collection it drops the **entry**, leaving a hole in the array
   instead of a missing aggregate. The `NOT NULL` + FK pair is what makes the choice safe.
3. **The ROOT join's fields are addressable in a criteria; the two CHILD joins' are not.**
   That asymmetry is the whole of §9's filter table: `?filter[tenantWorkspace][eq]=acme` is
   served, `?filter[groups.groupKey][eq]=engineering` is a typed 400. Narrowing a root by a
   field of a 1:N collection is a pushdown one root `SELECT` cannot express.
4. **Nothing about the write side moves.** A join field is not part of the `TableSchema`, so
   it never enters an INSERT or an UPDATE, and the entries still store one column each. The
   re-attach invariant is untouched precisely *because* the key is never persisted.
5. **Filled on load, EMPTY on a freshly added entry** — nothing traversed a foreign key for
   a struct the mapper built from a request body a millisecond ago. §3's
   `IsSameBusinessIdentity` note and §7's *"why the read joins do not answer U8–U13"* both
   depend on this.

**Why the id and not the key is stored**, one level up from `Role` and `Group` and for the
identical reason: archive is **one-way** on both `Group` and `Role`, so a retired one comes
back as a new row with a new id. An entry storing the *key* would silently re-attach to the
recreated row; one storing the **id** cannot.

**What the traversals do NOT bring: the target's `deleted_at`.** `Role` carried the
equivalent on its own grants and dropped it on 2026-08-24, and `Group`'s spec was amended to
match. Republishing a column the owning aggregate deliberately keeps off its own reads is a
back door to a decision already made the other way. So a read renders *what* this user
belongs to and says nothing about whether it is retired; U8/U9 answer that, and they answer
it with a probe.

**Why these three rules cannot be answered BY the joins** — the same argument
`../group/spec.md` §7 makes, and it holds twice here: U8, U9, U12 and U13 judge the entries
a write **adds**, and an added entry has no joined value. Reading `RoleKey` off it yields
`""`, which U12b would read as "no wildcard" — a security rule passing on a blank field, the
worst possible failure direction. Beyond that, the joins reach `groups` and `roles`, while
U12/U13 ask about the **permissions behind them**, two and three hops out.

### The password, end to end — the one thing in this entity that is not `Group`

Read this as one sentence: **the plaintext exists in memory for the length of one request
and is never written anywhere.**

```
request body            entity (in memory)                  row              response
────────────            ──────────────────                  ───              ────────
password         ─────► Password  (vos.Password)  ──┐        —                 —
passwordConfirm  ─────► PasswordConfirmation      ──┤ U5      —                 —
                                                    │
                        rules run: U5, U6, U7 ──────┘
                                     │
                                     ▼
                        PasswordHash ◄── Hasher.Hash(Password)   password_hash    — (hidden)
```

Six properties, each of which is a decision:

1. **The plaintext is validated by a value object, and the framework's automatic pass
   reaches it even though it is not persisted** — the pass "walks every exported top-level
   field by reflection" (verified above), and nothing about it consults the `TableSchema`.
   So `vos.Password` enforces §B Q3's policy on every write that carries one.
2. **On writes that carry no password, the pass is switched off per mode** with
   `r.IgnoreValueObject("Password")` inside the mode gate — the documented escape. Without
   it, a plain `PATCH /users/{id}` renaming somebody would answer *"password is required"*.
3. **The hash is computed in the application layer, not in the domain.** The domain says
   whether the plaintext is acceptable; a `PasswordHasher` port turns it into a hash. The
   domain never imports a crypto package and never learns which algorithm is in use — which
   is what makes Q4 a one-line change later.
4. **The hash is `hidden`**, so it is out of the by-id read, out of every listing row, out
   of the write results and out of the CSV/XLSX exports. This is the invocation's *"no
   endpoint e consulta, nunca volta a senha"*, enforced by the framework rather than by
   remembering to leave a field off a DTO.
5. **The hash is `assignedFrom: derived`**, so it is absent from every write request, every
   command and the OpenAPI request schema — there is no shape in which a caller can propose
   one, and nothing has to ignore a value that was sent.
6. **Nothing about the password reaches the audit trail as a value — but that takes a
   THIRD declaration, and this spec originally missed it.** See the section below; it is
   the one correction the model gate produced.

### Why the password must not reach the audit trail — and what actually stops it

*(Amended 2026-08-25, from the maintainer's question at the model gate. The first draft of
this spec asserted that the password never reaches the audit trail because the plaintext
fields are runtime-only. **That was half an answer**: it is true of the plaintext and false
of the hash.)*

**What the framework does by default, verified in `table-schema.html`:**

> Declaring a field persists it — and, by the same declaration, sends it to the outbox
> payload (and from there to the topic, to every consuming service and to the projected
> document) **and to the audit event**.

So `hidden` is not enough, and it was never claiming to be: `hidden` is the RESPONSE axis —
by-id, listing row, write results, exports. The audit event is a **different copy of the
row**, and `password_hash` would land in it as an ordinary field: masked from every API
caller and printed in full in `audit_events`, and — with `audit.destinations` at its default
— echoed by `slog` into whatever log aggregator the deployment ships to.

**Three reasons that is unacceptable here, in the order that matters:**

1. **The audit trail is permanent, and it is the one copy nobody rotates.** A row is written
   inside the write's transaction and is meant to be immutable forever. Every other place a
   hash could leak has a lifetime; this one does not. A parameter set that is strong in 2026
   is a weekend of GPU time in 2036, and the trail will still be there holding a hash for
   every password every user ever set.
2. **`slog` moves it out of the database's blast radius.** The default routing echoes the
   event to the log aggregator, which is a *second* system, with its own access control,
   its own retention, its own vendor and — usually — a far wider audience than the users
   table. A DBA-only secret becomes an everyone-with-Kibana secret, and nothing about that
   crossing is visible in the code that caused it.
3. **The audit event is read through an endpoint.** `audit.endpoint` mounts a read surface
   over these rows. Without redaction, "who may read the audit trail" silently becomes "who
   may read every password hash in the tenant" — an authorization edge nobody declared and
   nobody would find by reading the user routes.

**And the trail loses nothing by masking it.** What an auditor needs from a credential
change is *that it happened*, by whom and when — never the value. The framework's design
matches that exactly: **redaction runs AFTER the delta is computed**, so a changed field
keeps its entry and only its two sides are masked —

```json
{ "field": "PasswordHash", "fieldLabelKey": "UserPasswordField",
  "from": "***", "to": "***" }
```

— and the doc is explicit that *"an entry that is present means the field changed — read it
that way, because `from` and `to` being equal is the mask, not a no-op."* On an insert the
key is present and masked rather than dropped, so a reader never has to tell "hidden by
policy" apart from "this field did not exist in that schema version".

**The declaration, and both axes are mandatory** (a missing one is a construction panic —
the framework refuses to guess whether silence meant leak or mask):

| Axis | Redactor | Why |
|---|---|---|
| `core.InAudit(...)` | `core.RedactWith("***")` | the three reasons above |
| `core.InSync(...)` | `core.RedactWith("***")` | there is no broker today, but the axis is not optional and the honest value is the same one. When `/omnicore:configure` adds Mongo, the projected document and every consuming service inherit the mask **without this model changing** — and the doc notes the composer applies the same redactor, so a rebuild cannot reintroduce it |

`core.RedactKeepLast(n)` is deliberately **not** used: keeping the last four runes of a PHC
string discloses the tail of the hash and nothing a human wants. `core.RedactWith("***")`
on both axes is the whole declaration.

**What redaction does NOT do, stated because it is exactly the kind of gap this amendment
exists to close:** *"Nothing on the read side is refused because a field is redacted.
Filters, ordering, `?search=` and the aggregate DSL keep working."* The framework masks its
own copies; it does not decide policy on the read surface. Keeping `passwordHash` out of
every `filter:` and `sort:` tag (§9) is therefore a **separate** decision that has to be made
by hand, and §9's last table row is where it is recorded.

**Four places, four different mechanisms, and all four are required:**

| Copy | Kept out by | Failure if forgotten |
|---|---|---|
| the write request / OpenAPI request schema | `assignedFrom: derived` | a caller can set a hash directly, bypassing the policy and the hasher |
| every response body + the exports | `hidden` | the hash is handed to any caller who may read the row |
| the audit event (`audit_events`, the `/audit` endpoint, the `slog` echo) | **`redact.inAudit`** | a permanent, log-shipped, separately-authorized copy of every hash |
| the outbox payload → topic → consumers → projected document | **`redact.inSync`** | inert today (no broker); a leak the day the posture changes |
| a filter or `orderBy` over it | **nothing automatic** — simply never declaring it (§9) | `?filter[passwordHash][startswith]=…` walks the hash out one rune at a time |

**The plaintext needs none of this**, and that part of the original claim holds: `Password`,
`PasswordConfirmation` and `CurrentPassword` are runtime-only fields with no column, so the
persister never sees them and no copy of the row can contain them. The `PasswordHasher` port
must also never log its input — checked in §D's trap list, since it is the one place a
plaintext could still reach a log line.

### The new value objects

| VO | Kind | Rule | Notification |
|---|---|---|---|
| `vos.PersonName` | **composite**, hand-written (`written: manual`) | Spans `Given` + `Family`. Each part: 1–75 runes · at least one letter · no run of 4+ identical · trimmed and single-spaced. **No word count and no distinct-rune floor** — a half-name is routinely two runes (`Ng`, `Wu`, `Li`) and `DisplayName`'s anti-junk floor would reject every one of them. **Explicitly NOT `vos.DisplayName`**: that file's own header reserves `PersonName` for people, because a person's name follows different norms and a change made for it must not move every organization's bounds. Declares `IsValid` and the full-name method; **declares NO `Value()`** — that is what makes it a composite rather than a scalar | `InvalidPersonNameNotification`, raised per offending part so the caller is told **which** half is wrong |
| `vos.Email` | raw, hand-written | ≤ 254 runes total, local part ≤ 64 · exactly one `@` · a domain with at least one dot and no leading/trailing/doubled dot · no whitespace anywhere · **all-lowercase required, never silently lowercased** | `InvalidEmailNotification` |
| `vos.Password` | raw, hand-written | **DECIDED at the gate: 8–128 runes and ALL FOUR character classes** — lowercase, uppercase, digit and symbol, each at least once — plus: no leading or trailing whitespace, valid UTF-8, and Unicode classes (not ASCII), so `ç` counts as a letter and `£` as a symbol in every one of the seven languages this service serves | `WeakPasswordNotification` — deliberately ONE notification for every policy failure, mirroring `DisplayName`/`Description`, so a caller is told the policy once rather than four times |
| `vos.UserStatus` | enum, generated | `active` · `suspended`. Zero value is the Unknown sentinel and is never a member | `UnknownUserStatusNotification` |

**On `vos.Email` refusing uppercase instead of lowercasing it** *(proposed; alternative:
normalize at the boundary)*. It follows `Tenant.workspace`'s rule — *"no input
normalization — `" Acme "` and `"ACME-CORP"` are refused, never silently repaired"* — and
it is what keeps the global unique index honest: if `Maria@acme.com` were storable,
`maria@acme.com` would be a second user and the README's central invariant would be broken
by case alone. **The consequence, named:** the future `POST /auth/login` must lowercase what
the user typed *before* it looks the address up, because a human typing their address into a
login box will capitalize it. That is one line in a route that does not exist yet, and it is
recorded here so it is not discovered by a support ticket.

- **`domain.ID` is the right type at this pin** for `TenantID`, `GroupID` and `RoleID` —
  `table-schema.html`'s supported column shapes, already proven by `Role.TenantID` and
  `GroupRole.RoleID` in this project. Wire DTOs stay `string` and convert at the mappers.
- **`GroupKey`, `GroupName`, `RoleKey`, `RoleName` are plain `string`, not value objects.**
  A join field carries no domain type; the value belongs to the owning aggregate, arrives
  read-only, and reconstructing `vos.GroupKey` here would hand back an instance no rule of
  `Group` ever approved. Same decision `../group/spec.md` §2 made and for the same reason.
- **Uniqueness enforcement style: domain pre-check + DB backstop**, the project's
  established style (`TenantService.WorkspaceTaken`, `RoleService.RoleKeyTaken`,
  `GroupService.GroupKeyTaken`), so a duplicate address reports **together with** the other
  validation errors, with the partial unique index bound in the repository's `Constraints`
  map as the race backstop.

## 3. Children (1:N)

| Child | Of whom | Edit strategy | Restorable alone? |
|---|---|---|---|
| `user_groups` | the flat root (`users`) | **B — targeted per-child ops** (proposed; alternatives: A replace-all, C own aggregate) | **no** — so B is legal |
| `user_roles` | the flat root (`users`) | **B — targeted per-child ops** (proposed; same alternatives) | **no** |

- **Why B and not A**, one level up from `Group`'s argument and strictly sharper: a
  replace-all `PUT` means an omitted group or role is revoked, and here the blast radius is
  one person's entire access. A partial client that forgets a field de-provisions somebody.
- **A PAIR per collection, not the usual trio.** Neither entry has an editable field — its
  single column *is* its identity — so "update this entry" would turn entry A into entry B
  while keeping A's row id, which an audit trail reads as one grant *becoming* another
  instead of as two events. Four child operations in total:
  - **JOIN** `POST /users/:id/groups` — body carries `groupID`; the server mints the child id
  - **LEAVE** `PATCH /users/:id/groups/:childId/archive` — soft removal, **never `DELETE`**
  - **GRANT** `POST /users/:id/roles` — body carries `roleID`
  - **REVOKE** `PATCH /users/:id/roles/:childId/archive` — soft removal, **never `DELETE`**
- All four are commands **on the root** (load root → a domain method mutates the one child →
  the framework persists the diff) and all four dispatch `ModeUpdate`, so `IfInsertOrUpdate`
  in §7 covers them.
- **The by-id guard lives in a domain method on `User`**, not in a loop inside the command
  mapper: an absent child answers the canonical `RecordNotFoundNotification` (404), never
  the framework's `EntityDoesNotExistNotification` (422).
- **`IsSameBusinessIdentity` is over `GroupID` / `RoleID`, written explicitly.** Each entry
  carries three fields, two of which are read-only join values that are **blank on a
  freshly attached entry**; `domain.IsSameByBusinessFields` would compare all three and
  answer "different" for an attachment that duplicates a stored one — the duplicate guard
  failing open. This is `../group/spec.md`'s finding, and it applies twice here.
- **No per-child unarchive**, by framework construction. Re-joining a group the user left is
  a fresh JOIN with a fresh child id, which reads correctly in the audit trail.
- ⚠️ **The root-archive auto handler is instantiated exactly ONCE per surface.** Wiring it
  to any of the four child routes type-checks, boots, answers 200 — and archives the entire
  user. The canonical trap of this model, doubled here because there are four child routes
  instead of two; repeated in the web task.

## 4. Siblings (1:1)

**`N/A — no facet split off, and this IS the decision, not a skipped step.**

The model has three optional-ish fields — `PasswordChangedAt` (nullable),
`MustChangePassword` and, under Q2-B, `PasswordHash` — and they form an obvious candidate
group: a `user_credentials` satellite holding the credential, which is the shape §A-1 says
Okta ships and which would keep the secret out of the row every listing reads.

**It loses, on one hard constraint and one soft one:**

- **Hard:** a sibling forces the update shape to include **PUT** (§8's invariant — PATCH
  cannot assign null, and the root's PUT with the facet all-null is the only thing that
  clears a sibling row). This service is PATCH-only across all four existing entities;
  `User` would become the one entity speaking a second update verb, and it would do so to
  express "clear this user's credential", which is not an operation this model has.
- **Soft:** the facet is two or three scalars, not a bulky or genuinely sparse one. `hidden`
  already keeps the hash out of every response, so the satellite would buy no privacy the
  root does not already have — the reason to split would be *reads*, and a relational view
  loads a sibling with the root anyway.

Recorded so it is a considered decision: if §C-4 is ever taken (`lastLoginAt`,
`failedLoginAttempts`, `lockedUntil` — login-flow state written on every sign-in attempt,
including failed ones), **that** is the field group worth revisiting for a satellite, because
it is written far more often than the user row and for a different reason.

## 5. Modes                                             [required]

`Display, Insert, Update, Archive` — **one-way archive, no `Unarchive`** (proposed;
alternative: add `Unarchive` plus a rule refusing it when the address has since been
retaken).

The reason is the e-mail index, and it is specific to this entity rather than inherited:
archiving a user **releases their address** (the README says so, and the index is
active-only), so between the archive and any hypothetical unarchive, a new hire may have
been registered with it. An unarchive would then either fail on the unique index at the
worst possible moment, or — if the index were total instead — the address could never be
reissued, which the README rejects explicitly.

**Reversible deactivation has a home, and it is `status: suspended`.** That is the same
distinction `Tenant` draws: suspension keeps the row listed and stops the product;
archiving is removal. A user on leave is suspended; a user who has left the company is
archived. Getting this backwards — using archive for a leave of absence — is what the
`status` field exists to prevent.

**The two compose in one direction only, and U15 is what enforces it:** archiving **forces**
`suspended` (§7 U15, a mutation inside `IfArchive`), so an archived user is always
suspended. The reverse does not hold — activating a user does not unarchive them, and with
no `Unarchive` mode there is nothing to unarchive them with.

**The cost, named:** archiving the wrong person is not undoable through this API. They come
back as a new row that must be re-joined to their groups and re-granted their roles, and
their old row remains in the audit trail. Entra's 30-day restore window exists because that
mistake is common. If that case should win, the answer is `Unarchive` plus one rule refusing
it when the address is taken — one mode and one rule, addable later without rework.

## 6. Delete semantics                                  [required]

**Soft only — archive, no hard delete.** `PATCH /users/:id/archive`; there is no `DELETE`,
on the root or on either child. Consistent with all four existing entities: a purge would
destroy the only human-readable record of what a past access meant, which is precisely what
an access review reads, and tokens and audit rows already point at the user's UUID.

Per-child removal is `PATCH /users/:id/groups/:childId/archive` and
`PATCH /users/:id/roles/:childId/archive` — same verb-truth rule: `DELETE` means an
irreversible purge and nothing else.

## 7. Business rules                        [required]

| # | Field(s) | Rule | Verb scope | Notification | HTTP |
|---|---|---|---|---|---|
| **U0** | `TenantID` | **BARRIER.** Pull `domain.ID`'s own validation forward and END THE PASS if anything has already been rejected — `kind: valueObject`, `guard: true`, declared FIRST | `IfInsertOrUpdate` | the value object's own (`InvalidIDUUIDNotification`) | 422 |
| U1 | `TenantID` | Immutable after creation — a user never moves between tenants | `IfUpdate` | `UserTenantIsImmutableNotification` | 422 |
| **U1b** | `TenantID` | **Conditional source on create.** `*:*` superadmin → the body names it (required). Tenant caller → **inherited from the `tenant_id` claim**; a body value equal to the claim is accepted, a **different** one is refused (never silently overridden). No identity at all → the body, dev bench only | `IfInsert` | `domain.TenantMismatchNotification` (mismatch) · `domain.TenantMissingNotification` (no claim, no bypass) | 403 |
| U2 | `Email` | Unique **across the whole platform**, over active rows. Service pre-check + the partial unique index as backstop. **`IfInsert` only** — U2b freezes it, so there is no update to exclude-self against | `IfInsert` | `UserEmailAlreadyExistsNotification` | 409 |
| **U2b** | `Email` | **Immutable after creation** — it is the login handle and, once §C-2 lands, the delivery address | `IfUpdate` | `UserEmailIsImmutableNotification` | 422 |
| U3 | `TenantID` | The owner tenant must exist, not be archived and not be **suspended**; a `trial` tenant is a live customer and passes | `IfInsert` | `UserTenantDoesNotExistNotification` | 422 |
| U4 | `TenantID` | **Tenant isolation.** The row's tenant must equal the caller's `tenant_id` claim, unless the caller is a `*:*` superadmin | `IfInsertOrUpdate` + `IfArchive` | `domain.TenantMismatchNotification` (framework-owned, translated) | 403 |
| U5 | `Password`, `PasswordConfirmation` | **The two typings must match.** The invocation's confirmation requirement | `IfInsert` + both password operations | `PasswordConfirmationMismatchNotification` | 422 |
| U6 | `Password` | The policy of §B Q3 — length floor/ceiling, classes if Q3-B, no leading/trailing whitespace | — | **not declared here** — `vos.Password` validates by type on every write that carries one. Declaring `required` beside it would tell the caller the same thing twice | 422 |
| U6b | `Password`, `Email`, `Name` | **Context rule.** The password must not contain the e-mail's local part, nor any ≥4-rune word of `Name.Given` or `Name.Family`, case-insensitively | `IfInsert` + both password operations | `PasswordEchoesIdentityNotification` | 422 |
| **U7g** | `Email`, `CurrentPassword` | **CREDENTIAL BARRIER, on the open CHANGE only.** The address must resolve to a live user **and** the current password must verify — and the two are ONE question with ONE answer: *"usuário ou senha inválido"*. `guard: true`, declared FIRST, so nothing below it runs and no second notification can leak which half failed | the open change operation | `InvalidCredentialsNotification` | **401** |
| **U7i** | `LockedUntil` | **Runs INSIDE U7g, before the hash comparison.** A locked account is refused — and refused with the **same generic answer**, so a lockout is not a signal either | the open change operation | `InvalidCredentialsNotification` (the same one) | **401** |
| **U7h** | `FailedLoginAttempts`, `LockedUntil` | **On every U7g failure: increment; at 5, set `LockedUntil = now + 15 min` and reset the counter. On success: zero both.** A write that happens on a request that FAILED — the one place in this entity where that is true | the open change operation | — (a mutation) | — |
| U7b | `Password` | On both password operations: the new password must not verify against the stored hash | both password operations | `PasswordUnchangedNotification` | 422 |
| U8 | `Groups[].GroupID` | Every joined group must exist, be **active**, and belong to **this user's tenant** — one rule, one message | `IfInsertOrUpdate`, over the entries this write ADDS | `GroupNotAvailableInTenantNotification` | 422 |
| U9 | `Roles[].RoleID` | Every directly granted role must exist, be **active**, and belong to **this user's tenant** | `IfInsertOrUpdate`, over the entries this write ADDS | `RoleNotAvailableInTenantNotification` — **reused**, already declared and translated for `Group` | 422 |
| U10a | `Groups[]` | No duplicate group for one user | `IfInsertOrUpdate` | `UserAlreadyInGroupNotification` | 409 |
| U10b | `Roles[]` | No duplicate direct role for one user | `IfInsertOrUpdate` | `UserAlreadyGrantsRoleNotification` | 409 |
| U11a | `Groups[]` | At most **50** groups per user | `IfInsertOrUpdate` | `TooManyGroupsForUserNotification` | 422 |
| U11b | `Roles[]` | At most **50** direct roles per user | `IfInsertOrUpdate` | `TooManyRolesForUserNotification` | 422 |
| U12a | `Roles[]` | **No privilege escalation** — a caller may grant a role only if they hold **every** permission it grants. A `*:*` superadmin passes by construction | `IfInsertOrUpdate`, over ADDED entries | `CannotGrantRoleWithUnheldPermissionsNotification` — **reused** from `Group` | 403 |
| U12b | `Roles[]` | **No wildcard-bearing role** may be granted through the API | `IfInsertOrUpdate`, over ADDED entries | `CannotGrantWildcardRoleNotification` — **reused** | 403 |
| U13a | `Groups[]` | **No privilege escalation, three hops** — a caller may add a user to a group only if they hold every permission every role in that group grants | `IfInsertOrUpdate`, over ADDED entries | `CannotJoinGroupWithUnheldPermissionsNotification` | 403 |
| U13b | `Groups[]` | **No group carrying a wildcard-bearing role** may be joined through the API | `IfInsertOrUpdate`, over ADDED entries | `CannotJoinWildcardGroupNotification` | 403 |
| U14 | `Status` | Transitions `active→suspended`, `suspended→active` and no-ops only | `IfUpdate` | `InvalidUserStatusTransitionNotification` | 422 |
| **U16** | *(read side)* | **READ SCOPE — a tenant JWT sees ONLY its own tenant's users.** `ToCriteria(ctx)` injects `tenant_id` = the caller's claim, so the listing returns only them and a by-id read of another tenant's user answers **404, not 403**. A `*:*` superadmin skips the filter | `IfDisplay` — **not a `BuildRules` rule**: it lives in the query's `ToCriteria`, which is why it needs its own number instead of hiding in §10's prose | — | 404 (by id) · empty page (listing) |
| **U15** | `Status` | **Archiving forces `suspended`** — set in an `IfArchive` closure, not validated. A **mutation, not a rule that refuses** | `IfArchive` | — (a mutation, raises nothing) | — |
| — | `Name`, `Email`, `Status` | format, length, substance, membership | — | **not declared here** — `vos.PersonName`, `vos.Email` and `vos.UserStatus` validate by type on every write | 422 |

*(The U-series is numbered to read beside `../group/spec.md`'s G-series: U0–U4 are G0–G5
one level up, U8–U13 are G6–G10 doubled because there are two collections, and U5–U7 are
the password — the part that has no counterpart anywhere else in the service. U14/U15 have
no G-counterpart at all; they are `Tenant`'s rules 12 and 13 one entity over.)*

### U15 — archiving forces `suspended`

*(Instructed by the maintainer at the model gate, 2026-08-25. **DECIDED, not proposed.**)*

Verbatim the shape `Tenant` already ships — `internal/domain/tenant_rules_manual.go`, read,
its `IfArchive` closure assigning `e.Status = vos.TenantStatusSuspended` and raising
nothing. Four properties carry over unchanged, and each is a reason the rule is written this
way rather than another:

1. **It is a MUTATION, not a validation.** It sets the field and raises no notification. The
   alternative — refusing to archive a user who is still `active` — would make every archive
   a two-request dance (suspend, then archive) and would answer 422 on a request whose
   intent was never ambiguous.
2. **It makes `archived + active` unrepresentable rather than merely refused.** There is no
   state to validate against, no report that has to filter it out, and no path — through
   this API or through a future one — that can produce it.
3. **It reaches the ROW, not just the audit event**, because archive is an ordinary
   full-field write at this pin: it emits the same UPDATE every other verb does, with
   `deleted_at` riding along as one more column. This is a pin-dependent fact and it is why
   the rule can live in `IfArchive` at all.
4. **It cannot collide with U14.** U14 is `IfUpdate`, U15 is `IfArchive`, and the two modes
   never run together — so "suspended is not a legal transition from here" can never fire
   against the value this rule just wrote.

**Its one consequence, which §5 already leans on:** since §5 has **no `Unarchive`**, this
rule has no "comes back suspended" half to state — an archived user does not come back at
all. If `Unarchive` is ever added, U15 gives it the same property `Tenant` has: the user
returns **suspended**, because the stored value *is* `suspended` and nothing sets it back.
Reactivation stays a separate, separately-audited act, which is the wanted behavior and not
an oversight.

### U16 — the read scope, written out because it is easy to leave in prose

*(Maintainer at the gate: **"nas consultas, tu não listou, mas se JWT de tenant, só consegue
ver os próprios usuários."** Correct — it was in §10's prose and had no rule number, so it
did not appear beside the writes. Promoted.)*

**"Os próprios usuários" = the users of the caller's own tenant** — the same Layer 3 scope
`Role` and `Group` already ship, applied unchanged. Three behaviours, and the third is the
one that matters:

| Caller | `GET /users` | `GET /users/{id}` of another tenant |
|---|---|---|
| tenant JWT | only rows whose `tenant_id` equals the claim | **404** |
| `*:*` superadmin | every row, every tenant | the row |
| no identity (dev bench only) | scope stands down | the row |

**404 and not 403, deliberately.** A 403 confirms the id exists; a 404 says nothing about
who else is on the platform. Same reasoning that collapses U8/U9's three failure causes into
one message, and the same reasoning behind §E's generic 401 — this service answers
"you may not" and "it is not there" identically whenever telling them apart would leak.

**Why it needs its own number rather than living in §10.** It is **not** a `BuildRules`
rule: it is a filter injected in the query's `ToCriteria(ctx)`, so it never appears in the
rule table the write verbs are read from, and a reader auditing "what protects this entity"
by scanning §7 would not find it. It is also the only protection here that is **not** a
refusal — nothing is rejected; rows simply are not there. That is what makes it invisible in
a test that only asserts status codes.

**A user reading their OWN row** needs nothing extra: it is an ordinary by-id read that
their tenant scope already permits. `/users/me` — resolving the caller's `sub` so a client
need not know its own UUID — is §C-10, and it changes no rule.

**The rejected alternative, recorded:** writes-only isolation, with every authenticated
caller able to read every tenant's users. A user listing is the customer's staff directory
**including e-mail addresses** — leaking it across tenants is strictly worse than leaking the
org chart, which `Group` already refused.

### U0 — the barrier, and why it is rule ZERO

Inherited from `../group/spec.md` §7 unchanged, and it matters more here: **three** probes
in this entity are scoped by the owner (`EmailTaken` is not, but `GroupIsUnavailableInTenant`
and `RoleIsUnavailableInTenant` both are), so an owner that is empty or not a UUID would
reach a criterion against a UUID column and answer **500** on a request whose problem is
plain validation. `domain.ID` validates itself, the framework's own value-object pass runs
*after* the rules, and rules do not short-circuit — only `guard: true` ends the pass. It
must be declared FIRST in the verb's rule list.

### U1b — where `TenantID` comes from, and why this entity does NOT copy `Group`

*(Instructed by the maintainer at the model gate, 2026-08-25: **"se criado por um superAdmin,
aceitar o tenantId na criação; se for token com tenantId sem ser superadmin, então precisa
herdar do próprio token jwt do usuário do tenant o tenant do novo usuário."** DECIDED.)*

This is the **one place this entity deliberately diverges from `Role` and `Group`**, and it
is worth the divergence because it resolves a trade-off both of those specs recorded as a
loss. The history, from `../group/spec.md` §7:

- `Role` was first built with `assignedFrom: identity-claim` — the server filling the owner
  from the token. It was wrong twice: a server-assigned field is **absent from every write
  DTO**, so (1) a superadmin could never create inside another tenant, and (2) on a dev
  bench with auth disabled nothing filled it and the write died on an empty id.
- Both entities therefore moved to **"always in the body"**, with G5 refusing any value that
  is not the caller's claim. That works, and it makes every tenant admin send a UUID the
  server already knows, on every single create.

**U1b is the third option, and it is strictly better than either:** the field stays in the
DTO (so a superadmin can name a tenant), and it becomes **optional** for everyone else
(because the claim answers it). Four branches, all of them decided:

| Caller | `tenantID` in the body | Result |
|---|---|---|
| `*:*` superadmin | **required** | used as sent — this is what lets a platform operator create a user inside a customer's tenant |
| authenticated, has `tenant_id` | **absent** | **inherited from the claim** |
| authenticated, has `tenant_id` | present, **equal** to the claim | accepted |
| authenticated, has `tenant_id` | present, **different** | **403 `TenantMismatchNotification`** — never silently overridden |
| authenticated, **no** `tenant_id` claim | anything | **403 `TenantMissingNotification`** — fail closed |
| **no identity at all** (`auth.mode: disabled`) | required | used as sent — dev bench only, and framework-enforced to `APP_PROFILE=dev` |

**Row four is a decision, not an oversight.** Silently discarding a value the caller sent is
the "normalize instead of refuse" move this project rejects everywhere else — `Tenant`'s
workspace refuses `" Acme "` rather than trimming it, for the same reason. A caller who
names tenant B while holding tenant A's token has either a bug or an intent, and both
deserve an answer instead of a shrug. It also keeps G5's semantics intact, so the three
entities still agree on what a mismatch means.

**This is a candidate improvement for `Role` and `Group` too, and it is NOT applied here.**
Both are built and approved; changing them is `/omnicore:evolve-entity`'s job and needs its
own approval. Recorded in §C-12 so the divergence is a known one rather than a drift.

### U8 / U9 — the tenant anchor is the TARGET USER's tenant

*(Maintainer at the gate: **"e se for um tenant, ele só poder dar os grupos ou roles do
próprio tenant."** DECIDED — and already what U8/U9 say; written out because the anchor is
the part that can be got wrong.)*

Every group joined and every role granted must belong to **the user's `tenant_id`**, not to
the caller's. For a tenant admin the two are the same value by U1b, so the rule reads as
instructed. For a `*:*` superadmin they are **not** the same, and the user's tenant is the
right anchor: an operator creating a user inside `acme-comercio` may only give them
`acme-comercio`'s groups and roles — the bypass lets them act *inside* another tenant, never
*across* two. Anchoring on the caller instead would let a superadmin hand tenant B's role to
tenant A's user, which is the one cross-tenant leak this whole model exists to prevent, and
it would arrive through the account that is trusted most.

### U5, U6b and U7 — the password rules, and why they are domain rules

Every one of them could technically live at the web boundary. All four reasons they do not:

1. **One envelope.** A mismatch, a too-short password and a duplicate e-mail must arrive
   **together**, in the 422 the rest of this service already produces. A check at the edge
   short-circuits and shows the caller one problem at a time.
2. **Seven languages.** A domain notification resolves through the translation catalogs; an
   ad-hoc check in a handler produces an English string.
3. **Two entry points, one rule.** Create and both password operations enforce the same
   policy. Written once as a rule, it cannot drift between them.
4. **U7a and U7b need the stored hash**, which the entity has and the request does not.

**U7b is not "no password reuse" and must not be sold as such.** It refuses only the
*current* password, because that is the only hash the row holds. Real reuse prevention needs
a password-history table, which is §C-5 and is deliberately not in this model.

**U7g is a barrier for a reason that is easy to get backwards.** The generic message is
worthless if the *other* rules answer around it. Consider the open endpoint with a wrong
e-mail **and** a 6-character new password: without the barrier the caller receives
*"usuário ou senha inválido"* **plus** `WeakPasswordNotification` — and the second one only
exists because the request got far enough to be evaluated, which is a yes/no oracle on
whether the address is registered. `guard: true` ends the pass, so a failed credential
produces exactly one answer and nothing else. §E's ordering table is the full contract.

**U6b — why the context rule survives even under Q3-A.** NIST-4 forbids composition rules;
it does **not** forbid context checks — it recommends them, alongside the blocklist. A
password that is the user's own e-mail local part is the single most guessable credential
this service can issue, and refusing it is not a composition rule.

### What U8 / U9 / U12 / U13 judge — added entries, not the stored ones

Inherited from `../group/spec.md` §7 and `../role/spec.md` §7 for the identical reason. All
of them ask about **the act of granting**, and an entry already in the row was asked when it
entered. Re-judging stored entries makes unrelated writes hostages of the past: a group
archived after the user joined it would make the user impossible to **rename**, and an
operator who has since lost a permission could no longer even **remove** the other
memberships. Mechanically `domain.GetAddedItemsOf`, never `GetCurrentItemsOf`. An insert is
unchanged (every entry is added), and a LEAVE/REVOKE asks nothing, because it adds nothing.

### U13 — the three-hop check, and what it costs

This is the longest reach in the service, and the first rule here with no counterpart one
level down. Adding a user to a group confers **every permission of every role that group
carries**, so the caller must hold all of them.

```
user_groups.group_id ─► groups ─► group_roles.role_id ─► roles ─► role_permissions ─► resource:action
                                    (join declared              (join declared
                                     on GroupRepository)         on RoleRepository)
```

Resolution, and why it is not as expensive as it looks: `GroupRepository` already declares
the traversal into `roles`, so one read hands back the group with every attached role's id;
`RoleRepository` already declares the traversal into `permissions`, so one read per role
hands back every grant with `resource`/`action` already filled. A write joining one group
carrying five roles pays **1 + 5** reads and no id→key resolution step anywhere.

**Order is load-bearing, exactly as on `Group`:** U8 → U13b → U13a. U13b runs before U13a
and removes the panic input (`HasPermission` panics on any argument containing `*`), and it
must answer **true for an unresolvable group id too** rather than let an unknown id fall
through to the escalation question. U8 precedes both, so an unknown group is reported as a
missing group (422) and not as an escalation attempt (403). The same ordering applies to
U9 → U12b → U12a on the role collection.

**What U13b costs, stated plainly and consistently with the rest of the service:** the
platform's own superadmin **user** — the one belonging to the `*:*` group — cannot be
created through this API. It is seeded by migration beside the reserved platform tenant, the
`*:*` role and the superadmin group, in the same migration the README already owes.

### U8 / U9 — one notification for three questions, on purpose

A joined group or a granted role must be **(a)** in the catalog, **(b)** active and **(c)**
owned by this user's tenant, and all three collapse into one message —
*"This group is not available in your tenant, or is no longer active."* Separating them
would confirm to a caller in tenant A that a specific UUID is a live group in tenant B: an
existence oracle over a competitor's org chart. Same reasoning that makes a cross-tenant
by-id read answer 404 rather than 403.

### The domain service port

```
UserService (internal/domain/user_service.go) — plain values, no error, per the local pattern
  EmailTaken(email string, selfID domain.ID) bool                          // U2 — NOT tenant-scoped
  TenantIsUnavailable(tenantID domain.ID) bool                             // U3
  GroupIsUnavailableInTenant(tenantID, groupID domain.ID) bool             // U8
  RoleIsUnavailableInTenant(tenantID, roleID domain.ID) bool               // U9
  GroupGrantsWildcard(groupID domain.ID) bool                              // U13b
  RoleGrantsWildcard(roleID domain.ID) bool                                // U12b
  CallerLacksAnyPermissionOfGroup(groupID domain.ID) bool                  // U13a
  CallerLacksAnyPermissionOfRole(roleID domain.ID) bool                    // U12a
  PasswordMatches(hash, plaintext string) bool                             // U7a, U7b
```

- **`EmailTaken` takes no tenant**, and it is the only fact in this service that does not —
  the index is global. Written without a tenant parameter so it cannot be scoped by accident.
- Every fact is **named for the problem, never for the healthy state** — the generated suite
  stubs the service so each probe answers "nothing found", which is what lets a valid fixture
  through. A fact spelled `GroupIsAvailable` would read `false` under that stub and mean "the
  group is gone", turning a correct spec red on the day it is written. The convention `Role`
  established.
- **`CallerIsSuperAdmin` is deliberately NOT declared.** Verified dead on `Role` and skipped
  on `Group`: the generator answers the question itself from `authz.bypass`, in every command
  mapper and in both queries. Declaring it would put a second, hand-written answer beside the
  generated one, free to disagree.
- **`PasswordMatches` is the one fact that is not a query.** It is a pure CPU comparison
  through the same `PasswordHasher` port that produced the hash, placed on the service
  because the domain must be able to ask it from a rule without importing a crypto package.

### G5-equivalent — how the identity reaches the rules

Layer 2 in `authz-seams.html` terms, unchanged from `Group`: the command mapper — the only
layer allowed to read `ctx` — writes `RequestingTenantID` ← `ctx.Identity().TenantID()` and
`RequestingPrincipalIsSuperAdmin` ← `ctx.Identity().IsSuperAdmin()` onto runtime-only fields
that are not in the `TableSchema`. Never `HasPermission("*:*")` (it panics), never a
hand-read of `Claims["permissions"]` (the claim name is configurable).

### Absent identity vs absent claim — two states, not one

| State | When | Verdict |
|---|---|---|
| **No `Identity` at all** (`ctx.Identity()` is nil) | `auth.mode: disabled` — the middleware never populates one | **stand down** — U4, U12a and U13a do not fire |
| **`Identity` present, claim empty or insufficient** | any authenticated request | **refuse** — 403, fail closed |

Collapsing them makes the entity unusable on the dev bench. The safety of the first row is
the framework's boot guard, not this entity's promise: `AuthModeDisabled` is accepted only
under `APP_PROFILE=dev`.

## 8. Update shape                                      [required]

**PATCH only.** No sibling in §4, so the PUT-invariant does not bind, and all four existing
entities speak PATCH.

Patchable: **`GivenName`, `FamilyName` and `Status` — and that is the whole list.** Frozen:
`TenantID` (U1) and **`Email` (U2b)**. Not on the update body at any time: `PasswordHash`
(`assignedFrom: derived` + `hidden`), `PasswordChangedAt`, `MustChangePassword` and
`EmailVerifiedAt` (all server-written), and the whole plaintext
triple — those move through the two dedicated password operations, never through the root
body. `Groups` and `Roles` move through the §3 child ops.

**This is a security property, not a style choice:** there is no request shape reaching
`PATCH /users/{id}` in which a password field exists to be sent.

## 9. Surfaces & reads                       [required]

- **REST: yes** (OpenAPI documented) · **GraphQL: yes**, root verbs only (`users`, `user`,
  `createUser`, `patchUser`, `archiveUser`) — the four child operations and both password
  operations are **REST-only**, exactly as `Role` and `Group` ship their collection verbs.
- **gRPC: no** — available later through `/omnicore:implement`, no rework.
- **Exports (CSV/XLSX): no** — the call all four existing entities made. An
  access-review export is the plausible reason to want one; it is one flag. **If taken, note
  that `hidden` already keeps the hash out of the export** — the framework renders exports
  from the listing Response.
- **Integration events: no** — the posture has no broker, so publishing is unavailable.
  Recorded so it is not silently forgotten: `user.created` and `user.archived` are the two
  facts other services will eventually want.
- **Reads:** by-id + by-params.
- **Reserved read controls served:** pagination (`first`/`last`/`after`/`before`) ·
  `orderBy` · `?fields=` **yes** · `?onlyTotal` **yes** · `?includeArchived` **yes** ·
  **`?search=` no** — a relational-served view answers free text with a typed 400
  (`UnsupportedCapabilityNotification`), so declaring it would advertise a capability the
  server refuses. Named because a user listing is the one place an operator most wants to
  type a name fragment; `contains`/`icontains` on `name` and `email` is what serves that
  need here.
- **Computed read fields: one — `name`.** `givenName + " " + familyName`, derived per row,
  **no column**. It exists because dropping the standalone `name` must not force every client
  to concatenate: the display string is served ready, from the same place on every surface
  (`FromQueryResult`, so REST, GraphQL and any future export agree). Its three properties are
  the framework's: `?fields=name` fetches the two sources, **`?orderBy=name` is a typed 400**
  (nothing to sort on), and **a filter over it is impossible** — which is exactly why
  `givenName` and `familyName` are both filterable in their own right.
  *(The other obvious candidate — the user's effective permissions, the union of both paths —
  is deliberately NOT a computed field: it is a per-row fan-out of two joins the read side
  cannot express, and it is the token issuer's job. §C-9.)*
- **Field-level read authz (`ReadCriteria.Restrict`): none.** `hidden` already
  removes the hash from every surface for every caller, which is stronger than restricting
  it, and every remaining field a caller may see the row for at all, they may see entirely.
- **View backing: relational** — `query.RelationalView("users", repo.Loader)`, contributed
  through the feature's `RelationalViews()` opt-in. The project posture, and the only option
  without Mongo. It takes its schema from the loader, carries no `Version`, no registry row,
  no rebuild and no Mongo collection.
- **The view inherits the read joins and declares nothing.** `TenantWorkspace`,
  `TenantStatus`, `GroupKey`, `GroupName`, `RoleKey` and `RoleName` reach the served document
  because `UserRepository` declared the traversals, not because the view asked.
- **`?fields=` reaches the joined values under `groups.groupKey`, `groups.groupName`,
  `roles.roleKey`, `roles.roleName`** — a child join's fields are addressed as
  `<segment>.<field>`. A path this read model does not have is a **400** naming the offending
  Go path, never a silent `200 {}`.
- **Pagination is a camouflaged offset** on this backing: the wire contract matches a Mongo
  view, but each cursor carries an absolute row index, so a row inserted ahead of the window
  shifts every later page by one. Recorded so a bulk export walk knows what it pages over.
- **The one real cost, stated up front:** a caller **cannot filter or sort by a group or a
  role**. `?filter[groups.groupKey][eq]=engineering` is a typed 400 — the read join renders
  the membership, it does not make it addressable. **"Who is in Engineering?" is not
  answerable from this listing**, which is §B Q1's cost restated where it will be
  rediscovered. It becomes answerable the day the service gains Mongo, with no change to this
  model.

- **Filter/sort operators per field** (low-risk — decided):

| Field | `filter:` | `sort:` |
|---|---|---|
| `tenantID` | `eq,in` | `asc,desc` |
| `givenName` | `eq,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `familyName` | `eq,in,startswith,istartswith,contains,icontains` | `asc,desc` — **the sort an operator actually wants**, and the reason the split earns its keep |
| `email` | `eq,ne,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `status` | `eq,in` | `asc,desc` |
| `emailVerifiedAt` | `eq,gte,lte` | `asc,desc` |
| `name` *(computed)* | **none — impossible** | **none — a typed 400**, it has no column to sort on |
| `mustChangePassword` | `eq` | — |
| `passwordChangedAt` | `gte,lte` | `asc,desc` |
| `tenantWorkspace` *(join)* | `eq,in,startswith,istartswith,contains,icontains` | `asc,desc` |
| `tenantStatus` *(join)* | `eq,in` | `asc,desc` |
| `createdAt` · `updatedAt` *(managed)* | `gte,lte` | `asc,desc` |
| `passwordHash` | **none — not declared anywhere** | **none** |

`passwordHash` appears in this table only to record that its absence is deliberate. Filters
and ordering are declared on the Request DTO, so a field is queryable **only** if something
declares it; nothing declares this one, on any surface, ever. A filter over a password hash
is an oracle — `?filter[passwordHash][startswith]=$argon2id$v=19$m=19456,t=2,p=1$AAAA` walks
it out one character at a time.

**And this is the one protection nothing automatic provides.** `hidden` governs responses
and `redact` governs the framework's own copies, but the docs are explicit that *"nothing on
the read side is refused because a field is redacted — filters, ordering, `?search=` and the
aggregate DSL keep working exactly as they do for any other field."* The framework offers
the mechanism and does not decide the policy. So the empty cells above are the policy, and
they hold only for as long as nobody adds a `filter:` tag to that field.

`passwordChangedAt` **is** filterable and sortable, deliberately: *"every user whose
credential predates the incident"* is the report that matters after a breach, and it is the
only reason this column exists.

## 10. Authorization                          [required]

### Layer 1 — the permission gate

`user:read` · `user:insert` · `user:update` · `user:archive` · **`user:grant`** ·
**`user:reset-password`** — the `<resource>:<verb>` taxonomy every entity here declares, plus
the fifth verb `Group` established and a **sixth**, taken at the gate.

**`user:reset-password` is a sixth verb because a password reset is a credential-changing
operation performed on somebody else.** Folding it into `user:update` would mean everyone
who can fix a typo in a name can take over any account in the tenant — the escalation this
model refuses everywhere else, arriving through the least-guarded door. Entra and Okta both
gate a reset behind a role a plain user-administrator does not have, for exactly this reason.
It is **an additional acceptor, not a replacement**: the reset accepts *self* **or** `*:*`
**or** this permission, so a tenant can run its own helpdesk while the self-service path
never depends on a grant somebody has to remember to make. **Its deployment cost, named:**
like `group:grant`, it is a catalog row somebody has to insert, and until they do, only the
user themselves and a platform operator can reset a password once `auth.authorization` is on.

| Operation | Route | Permission |
|---|---|---|
| create | `POST /users` | `user:insert` |
| patch | `PATCH /users/:id` | `user:update` |
| archive | `PATCH /users/:id/archive` | `user:archive` |
| list | `GET /users` | `user:read` |
| by id | `GET /users/:id` | `user:read` |
| join a group | `POST /users/:id/groups` | **`user:grant`** |
| leave a group | `PATCH /users/:id/groups/:childId/archive` | **`user:grant`** |
| grant a role | `POST /users/:id/roles` | **`user:grant`** |
| revoke a role | `PATCH /users/:id/roles/:childId/archive` | **`user:grant`** |
| **change own password** | `PATCH /users/password` | **PUBLIC — no token, no permission.** `auth.publicRoutes`; the current password is the credential. §E |
| **reset a password** | `PATCH /users/:id/password-reset` | **`sub` == `:id` · OR `*:*` · OR `user:reset-password`** |

**`user:grant` covers both collections, and that is a decision.** Splitting it into
`user:grant-group` and `user:grant-role` would let a deployment delegate the two edges
separately — but they confer the same kind of thing (permissions, by indirection) and the
group edge strictly dominates the role edge, so a principal trusted with one is trusted with
the other in every scenario worth modelling. One verb, following `group:grant`'s precedent.

**The two password operations are gated differently from everything else here**, and each
gate is the decision made at the model gate:

- **the CHANGE is public**, because the caller may be unable to obtain a token at all. §E is
  the whole contract, and it is the only unauthenticated write in this service.
- **the RESET accepts THREE acceptors: `sub` == `:id`, OR `*:*`, OR `user:reset-password`.**
  The self half must never depend on a grant — every user has to be able to rotate their own
  credential, and gating that on a permission lets an operator lock a whole tenant out of
  their own passwords simply by never making it. `*:*` is what makes platform support
  possible. And `user:reset-password` is what lets a tenant run its own helpdesk without
  whoever edits a name being able to take an account.
- **`user:update` explicitly does NOT reach either.** Whoever can fix a typo in a name must
  not be able to take over an account; that is the escalation this model refuses everywhere
  else, arriving through the least-guarded door.
- **Neither is on GraphQL.** REST only, like every other non-root verb here — and the public
  one especially: `auth.publicRoutes` matches an exact `METHOD /path`, and a single GraphQL
  endpoint cannot be made selectively public per field.

### Layer 2/3 — data access

**Every row is tenant-scoped, on READS and on WRITES both** — `Group`'s answer, applied
unchanged.

- **Writes (Layer 2):** U4. The row's `tenant_id` must equal the caller's claim, else 403.
  A `*:*` superadmin bypasses it. **On insert, U1b comes first and usually makes U4 a
  no-op**: a tenant caller does not supply a tenant at all, they inherit one, so there is
  nothing to mismatch. U4 still fires for the caller who supplies a *different* one, which
  is why U1b refuses rather than overrides.
- **The bypass acts INSIDE another tenant, never ACROSS two.** A `*:*` operator creating a
  user in `acme-comercio` names that tenant (U1b) and may then give the user only
  `acme-comercio`'s groups and roles (U8/U9, anchored on the **user's** tenant). There is no
  request shape in this entity that mixes two tenants' rows.
- **Reads (Layer 3):** `ToCriteria(ctx)` injects
  `crit.Filter["tenant_id"] = ctx.Identity().TenantID()`, so a listing returns only the
  caller's own tenant's users, and a by-id read of another tenant's user answers **404
  rather than 403** — it does not exist for this caller. A `*:*` holder skips the filter,
  detected with `ctx.Identity().IsSuperAdmin()`.
- **The cross-tenant group/role attach (U8/U9) is the same policy at the child level**, and
  the single-notification decision is what keeps it from becoming a read oracle.
- **A caller reading their OWN row** is served by the ordinary by-id read, which their
  tenant scope already permits. A `/users/me` convenience route is §C-10, not this spec.
- The rejected alternative, recorded: writes-only isolation. A user listing is the customer's
  staff directory including e-mail addresses; leaking it across tenants is worse than leaking
  the org chart, which `Group` already refused.

### The service-wide switch this entity depends on

**Neither profile configures `auth.authorization` today** — `dev` is `auth.mode: disabled`
and `prd` is `mode: jwt` with no `authorization:` block, which defaults to off. So
`RequirePermission` currently no-ops across the whole service and everything in this section
is generated, correct and **inert** until that switch is turned on; the rules fail closed the
moment it is. Flipping it is a service-wide posture change owned by `/omnicore:configure`.

**One consequence specific to this entity, and it is the sharpest one in the service:** with
authorization off, the self-service password change has no identity to compare `:id`
against. Under `auth.mode: disabled` (dev only, framework-enforced) that route must **stand
down to the same "no identity → do not fire" rule** as U4 — which on a dev bench means
anybody can change anybody's password. That is correct for a dev bench and catastrophic
anywhere else, and it is safe only because the framework refuses `disabled` outside
`APP_PROFILE=dev`. Written here so nobody "fixes" it by making the route work without an
identity in production.

---

## §E — The open change-password endpoint

*(Its own section because an unauthenticated write into a credential table is a security
surface, not a table row. Decided by the maintainer at the gate; everything below is the
consequence.)*

### What it is

```
PATCH /users/password              ← no :id, no token
{
  "email":                "maria@acme.com",
  "currentPassword":      "…",
  "password":             "…",
  "passwordConfirmation": "…"
}
```

**This is a login endpoint that happens to also write.** It takes an address and a password,
decides whether they are authentic, and answers. Every attack that applies to `POST
/auth/login` applies here, and the fact that this service does not have a login route yet
means **this endpoint arrives first** — the threat model cannot be deferred to the route
that comes later.

### Three mechanical facts that shaped it, all verified at the pin

| Fact | Consequence |
|---|---|
| `auth.publicRoutes` is an **exact `METHOD /path` match, no globs** (`yaml-reference.html`) | a public route **cannot carry a path parameter**. `PATCH /users/{id}/password` is not declarable as public at all — the by-email design is forced, not preferred |
| `authorization.enabled: true` runs a **boot scan requiring `RequirePermission` on every non-public route** (`yaml-reference.html`) | this route must be in `publicRoutes` or the service **fails to boot** the day authorization is switched on. It cannot be forgotten quietly |
| An anonymous request surfaces as `actor: "anonymous"` in the slog line and **NULL in the audit row's actor column** (`auth-middleware.html`) | the audit trail records the change with **no actor**, by construction. That is honest — nobody authenticated — and it is why §C-13 offers recording the source IP instead |

### ✅ The path — `PATCH /users/password`, and the collision it carries

**DECIDED at the gate: keep it inside the `/users` resource.** The credential belongs to the
resource it changes, and the route reads as what it is.

**The collision is real and it does not go away by being decided.** `PATCH /users/password`
and `PATCH /users/:id` are **the same shape to a router**: `password` matches `:id`. Which
one wins depends on **registration order**, and that is the kind of dependency that survives
every review and breaks on the first refactor that reorders a mount function.

Three things make it safe, and all three are acceptance checks in §D rather than good
intentions:

1. **`PATCH /users/password` is registered BEFORE `PATCH /users/:id`**, in the same mount
   function, with a comment on the line saying why the order is load-bearing.
2. **The by-id route validates `:id` as a UUID**, so even if the order were ever inverted,
   `password` fails to parse as an id and cannot silently become a patch against a
   nonexistent user. Defence in depth — order plus type.
3. **A test asserts it**, and it asserts the failure direction: `PATCH /users/password` with
   a valid body must reach the change handler, and must **not** answer 404 or 401. A 401
   there is the tell that `/users/:id` swallowed it and the auth middleware got there first.

*(The alternative, recorded: `PATCH /auth/password`, outside the id space entirely — cannot
collide, and it is where the framework's own docs put credential routes. Declined at the
gate in favour of keeping the credential on its resource.)*

### The generic answer — what it covers and what it must not

**One message, one status, for every credential failure:** *"usuário ou senha inválido"* /
`InvalidCredentialsNotification`, **401**. It covers all five of these, indistinguishably:

| Situation | Why it must not be distinguishable |
|---|---|
| the address is not registered | otherwise the endpoint is a **user-enumeration oracle** over the whole platform — and the index is global, so it enumerates *every tenant's* staff directory |
| the current password is wrong | the classic; distinguishing it confirms the address exists |
| the user is **archived** | confirms the address once existed |
| the user is **suspended** | confirms the address exists AND leaks an HR fact ("this person is deactivated") to anybody who guesses the address |
| the user's **tenant** is archived or suspended | confirms which customers are delinquent, to anyone who knows one employee's address |

**What it must NOT swallow: the new password's own policy failures.** A caller told
*"usuário ou senha inválido"* for a 6-character new password can never succeed and will
never know why. So the two classes of answer coexist on one endpoint — and the ordering is
what keeps that safe.

### The rule order — and it is load-bearing

| # | Rule | Answer | Continues? |
|---|---|---|---|
| 1 | **U7g** — the credential barrier: address resolves to a live, unsuspended user in a live tenant, **and** the current password verifies | 401, generic | **NO — `guard: true` ends the pass** |
| 2 | U5 · `vos.Password` (U6) · U6b · U7b — the new password's shape, context and difference | 422, specific, **reported together** | yes |

**Reversing these two is the bug this table exists to prevent.** With the policy rules
running first, a request carrying an unregistered address and a weak new password receives
*"usuário ou senha inválido"* **plus** `WeakPasswordNotification` — and the presence of that
second notification is a yes/no oracle on whether the address is registered, delivered by
the very message that was supposed to hide it. `guard: true` is what makes the generic
answer actually generic.

### Timing — the leak the generic message does not close

If the address is unknown, the honest implementation does no work and answers in ~1 ms. If
it is known, it runs Argon2id at the OWASP baseline and answers in ~100 ms. **That is a
100× timing oracle, and it enumerates users just as well as a distinct message would.**

**The mitigation is mandatory, not optional: on an unknown address, verify the supplied
password against a fixed dummy hash before answering** — the same Argon2id parameters, a
constant hash generated at boot. The call is thrown away; its cost is the point. This is
standard practice in every credential-verifying endpoint and it is the single most-missed
line in one.

### Lockout — ✅ TAKEN at the gate

*(The first draft deferred this. The maintainer took it, and it is the right call: §C-4's
whole reason for deferring — "nothing would write these columns" — stopped being true the
moment this endpoint existed.)*

| | |
|---|---|
| Columns | `failed_login_attempts INT NOT NULL DEFAULT 0` · `locked_until TIMESTAMPTZ NULL` |
| Threshold | **5 consecutive failures → locked for 15 minutes** *(a calibration, one constant each, changeable without a migration)* |
| On failure (U7h) | increment; at the threshold set `locked_until` and zero the counter |
| On success | zero both, in the same write that changes the password |
| While locked (U7i) | **the same generic 401** — a lockout that announced itself would be the enumeration oracle the generic message exists to close, and it would tell an attacker exactly when to come back |
| On the wire | **never.** Both fields are `hidden`; a caller cannot read how close an account is to locking, and a listing does not publish which accounts are under attack |

**The one property that makes this unlike every other rule in this spec: U7h writes on a
request that FAILED.** Every other write here happens because the domain accepted the
operation. This one is a counter that must survive the refusal, which is exactly why it is a
mutation on the entity rather than a notification — and why §D flags it as hand-written: no
rule DSL has a word for "persist this while rejecting the request".

**What it does NOT cover, said plainly:** an attacker spreading attempts across many
addresses, or across many source IPs against one address more slowly than the window.
Per-IP throttling at the edge is the complement, it costs nothing in this model, and it is a
deployment concern rather than a spec one — recorded here so "we have lockout" is not read as
"we are done".

> ### ✅ ANSWERED: **A — the lockout columns, now.** B alone protects against volume, never
> against a slow distributed attempt, and leaves no record that anything was tried. Per-IP
> throttling at the edge stays available as a complement and is a deployment concern.

### What else the endpoint must do

- **Enforce the full policy on the new password** — U5, U6, U6b, U7b. Nothing about being
  public relaxes it.
- **Clear `mustChangePassword`** and stamp `passwordChangedAt`. This is the route that
  resolves the state a create or a reset set.
- **Not be tenant-scoped**, and it cannot be: there is no claim. The **global** unique index
  on `email` is what makes one address resolve to exactly one user platform-wide, with
  nothing to scope by. This is the README's central invariant paying for itself.
- **Stand down U4 (tenant isolation) by construction.** There is no identity, so there is no
  claim to compare — the "absent identity → stand down" branch of §7. **This is the one route
  in the service where that branch is reachable in PRODUCTION**, by design and not by the
  `auth.mode: disabled` accident. Written here so nobody "fixes" it later by requiring a
  claim the caller structurally cannot have.
- **Never appear in `?fields=`, a filter or a response.** It writes; it returns 204 or a
  minimal body. It must not answer with the user's row, which would turn a credential check
  into a profile read.

## §C — Improvement suggestions (offered, not baked)

**None of these is in the model above.** Each is a separate yes/no and each is additive.

1. **`externalId` — the SCIM/directory-sync handle.** Same offer `Group` recorded and
   declined: one nullable `VARCHAR(255)` plus a unique-per-tenant index. Users are the
   object enterprise customers sync hardest, so if IdP provisioning is on the roadmap this
   is the cheapest moment. **Recommendation: skip now**, take it together with `Group`'s
   when provisioning becomes real — they should arrive as a pair.
2. **E-mail verification (`emailVerifiedAt` + a token flow).** Real and eventually
   necessary; it needs a token store and an outbound mail path, neither of which exists.
   **Deferred, recorded**, and it is what §B Q6 leans on: a verified-address model changes
   an e-mail *change* from "instant" to "pending re-verification".
3. **`givenName` / `familyName` beside `name`.** Buys sort-by-surname and salutations. The
   reason it is not baked: it is trivially additive later (two nullable columns), and
   imposing it now forces every caller to split names that many cultures do not split.
4. **~~Login-flow state~~ — SPLIT at the gate.** `failedLoginAttempts` and `lockedUntil` are
   **IN** (§E, U7h/U7i): the deferral rested on "nothing would write them", and the open
   change-password endpoint is exactly that writer. **`lastLoginAt` stays deferred** — it
   really does wait for a login route, and nothing here signs anybody in. §4 still records
   that this is the field group worth a 1:1 satellite if it grows, because it is written on
   every attempt including the failed ones; at two columns it is not there yet.
5. **Blocklist + password history.** NIST-4 requires the blocklist (§A-11) and it is the
   single highest-value password control after length. Both need something this repository
   does not have: a breached-password corpus (the Pwned Passwords k-anonymity API, or a
   local list), and a `user_password_history` table with a retention rule. **Recommendation:
   take the blocklist when there is a corpus to check against**; a hand-written 20-entry list
   is worse than none, because it reads as compliance.
6. **A pepper.** Covered in §B Q4. If it is ever wanted, **now is cheaper than later** —
   adding one afterwards leaves every stored hash unverifiable until each user next signs in.
7. **`locale` / `timezone`.** This service already ships seven catalogs and resolves the
   language at the boundary; a per-user preference is where it eventually comes from.
   Two nullable columns, additive, no rush.
8. **MFA.** Out of scope entirely, and worth naming for one reason: **it is what makes §B Q3
   cheaper**. NIST-4 permits an 8-character minimum once the password is not the only
   factor. The framework's `Issuer` does not mint MFA state; this would be a new aggregate.
9. **Effective-permission resolution.** The README lists it as not started, and it is what
   the token issuer needs on every login: the union of the group path and the direct path,
   deduplicated. Not a computed read field (§9 says why) — a domain service question or a
   `ComposedView` once Mongo exists. **The single most useful thing to build after this
   entity**, because nothing else turns this model into a token.
10. **`/users/me`.** A convenience route resolving the caller's own subject to their row, so
    a client does not have to know its own UUID. One hand-written query handler; no model
    change.
11. **Self-protection rules.** Two cheap guards this spec does not declare: a user may not
    archive or suspend **themselves**, and a user may not remove **their own** last group or
    role. Both prevent an operator locking themselves out of the tenant. Cheap to add, and
    left out only because neither is a data-integrity rule.
12. **Backport U1b to `Role` and `Group`.** Both make every tenant admin send a `tenantID`
    the server already knows, on every create, because when they were built the only
    alternative (`assignedFrom: identity-claim`) broke the superadmin case — their specs say
    so. U1b's conditional source fixes that without losing anything, and after this entity
    ships, `User` will be the only one of the three that behaves well. **Not applied here:**
    both are built and approved, so it is `/omnicore:evolve-entity`'s job with its own
    approval. Recorded so the divergence is a known one rather than drift.
13. **Record the source IP on the open endpoint.** §E notes that an anonymous request lands
    in the audit row with a NULL actor — correct, and not very useful when the row being
    changed is a credential. The remote address is what an incident responder actually reads.
    It is not an entity field; it belongs on the audit event or a dedicated attempt log, and
    it pairs naturally with §C-4 / §E's option A.

## §D — What generation will have to write by hand, and why

**Read this before answering gate 1d.** The password is the first thing in this service the
generator's spec language cannot express, and the honest accounting matters more than usual.

### The gap, stated precisely

**Re-checked against `omnicore-gen 0.39.0`** (the build the maintainer installed at the
model gate), by running `explain keys` and `explain coverage` on both builds and diffing
them. One capability landed and it removes work from this list — **`redact` is now a
first-class spec key** (`fields[].redact.inSync` / `.inAudit`, with
`kind: plain | fixed | keep-last | hook`), so §2's redaction of `PasswordHash` is
**generated, not hand-written, and not adopted**. On 0.38.0 it would have been a hand-edit
to the schema file, which is the worst kind — a `TableSchema` is regenerated on every run.

The gap below is **unchanged** between the two builds; the wording is byte-identical:

### ✅ CLOSED at `omnicore-gen` 0.40.0 — both asks landed

*(2026-08-26. The maintainer took the report upstream and fixed it; the spelling is the one
proposed, `claim` kept as the default so no existing spec moved.)*

| Ask | Landed as | Verified |
|---|---|---|
| the body-fed, non-persisted field | **`runtime: true` + `source: body`**, with **`modes: [insert]`** naming which write verbs carry it (omitted = every write verb the entity has; `update` covers both update shapes, because the rule gates cannot tell them apart either) | a probe spec declaring one passes `check` with no blocker |
| the conditional owner source (the secondary ask) | **`assignedFrom: identity-claim` + `bypassMaySet: true`** — the server reads it off the caller's identity, and *"the caller who crosses the ROW SCOPE states this value instead"* | `explain coverage` now lists *"a server-assigned scope that yields to the bypass"* |

**And the guarantee this spec most wanted is now stated by the tool itself**, in
`explain vocabulary`: a `source: body` field *"crosses the write DTO, the command and the
entity for a rule to check, and **NO column, payload, audit event or response ever sees
it**"*. That is §2's four-copies rule, enforced by the generator instead of by four separate
declarations plus vigilance — the plaintext and the confirmation now cannot leak into a
payload or a trail even by mistake, because there is no column for them to be redacted FROM.

**What this changes in this spec: two rows leave the hand-written list, and U1b stops being
a hand-written mapper.** Everything else below stands. The record of how the gap was
established is kept, because it is what the fix was written from.

### How the gap was established — by running the tool, not by reading its documentation

*(The maintainer challenged this at the gate — "the idea is simple: put it on the entity, the
DTO and the command, and just leave the confirmation out of the TableSchema; is THAT what the
generator cannot do?" It is, and the first draft of this section argued it from the key
documentation. Below is the same claim re-established from `omnicore-gen check` output on
three probe specs. **The design he described is exactly what §2 specifies; what follows is
only about whether the generator has a word for it.**)*

| Probe | `omnicore-gen 0.39.0 check` answers |
|---|---|
| a field with `runtime: true` and no `claim` | ✗ blocker — *"a runtime-only field does not say which claim it comes from → name it, e.g. `claim: email`"* |
| a field with neither `runtime` nor a column | ✗ blocker — *"the column name is required"* |
| a field with `runtime: true` **and** a `claim` | ✓ accepted |

**So the field model has exactly two states — persisted (a column is mandatory) or
runtime (a claim is mandatory) — and no third.** The third is the one this entity needs:
present on the entity, on the request DTO and on the command, absent from the
`TableSchema`.

**And a claim would not rescue it either.** A runtime field is filled *in the command mapper
from the identity*, never from the body — verified against this project's own generated code:
`Group`'s `RequestingTenant` appears on the domain entity and in every command mapper and in
**no request DTO at all**. Giving the password an invented claim name would produce a field
the caller cannot send.

**There is no manual escape for a field.** `kind: manual` exists for a value object and
`rules.manual` for a rule; `explain keys` has no equivalent for `fields[]`. So the three
credential inputs have no hook file to live in, and a regeneration would erase them from any
emitted file they were added to by hand — which is what makes option **iii** below the worst
of the three rather than merely inelegant.

### The gap in one sentence

`omnicore-gen 0.39.0` has exactly one spelling for a field that is not persisted:

> `fields[].runtime` — *"Runtime marks the field as runtime-only: never persisted, **fed from
> the caller's token** (see `claim`), existing only for the rules to read."*

There is **no spelling for a runtime field fed from the request BODY**. That is precisely
what `Password`, `PasswordConfirmation` and `CurrentPassword` are: values a caller sends,
that the rules must read, that must reach the seven-language 422 envelope, and that must
never touch a column. A password/confirmation pair is the canonical instance of that shape —
it is not an exotic requirement, it is what every credential-holding entity needs.

**Per the repository's own rule, this is reported upstream, not routed around.** It is a
generator gap with a clean shape (`runtime: true` + a source of `body` instead of `claim`,
or a `transient: true` sibling key), and the maintainer owns `omnicore-gen`. Three ways
forward, and **choosing between them is part of gate 1d**:

| | What happens | Cost |
|---|---|---|
| **i — fix the generator first** | add the body-fed runtime field to the spec language, then generate this entity normally | the whole entity generates; the gap closes for every future credential entity |
| **ii — generate without the credential, hand-write the password path** | the spec YAML declares `PasswordHash` as `assignedFrom: derived` + `hidden` and says nothing about the plaintext; the create command's mapper, both password operations and the three transient fields are written by hand into hook files | the generated tree stays clean and re-generable; ~4 hand-written files |
| **iii — generate, then `adopt`** | generate, then hand-edit and `omnicore-gen adopt` the affected files | **worst of the three** — an adopted file stops tracking the spec forever, and the files affected are the insert command and the entity itself |

**Recommendation: ii if the entity is wanted now, i if the gap is worth closing first.**
They compose: ii today, and this entity re-generates cleanly once i lands.

### Everything else that is hand-written

| Written by hand | Why the generator cannot | Where it lands |
|---|---|---|
| `vos.PersonName` · `vos.Email` · `vos.Password` | `kind: manual`. The shapes are partly regex, but the substance checks are the project's shared anti-junk predicates, which are not statable as a pattern; `vos.Password`'s policy is a composition | `internal/domain/vos/` |
| U3, U6b, U7a, U7b, U8, U9, U12a/b, U13a/b | `rules.manual`. Every one needs a cross-aggregate probe or the hasher, which the rule DSL cannot phrase | `internal/domain/user_rules_manual.go` |
| **U15** (archive forces `suspended`) | `rules.manual` — it is a **mutation**, and the rule DSL states refusals, not assignments. Declared in the spec YAML as an id + description + `scope: [archive]`, written in the hook file. **`Tenant` did exactly this** (`specs/omnicore-gen/tenant.omnicore.yaml`, rule `archive-forces-suspended` → `internal/domain/tenant_rules_manual.go`), read and mirrored | `internal/domain/user_rules_manual.go` |
| The nine facts behind them | `kind: manual`. Each asks about `tenants`, `groups`, `roles` or the hasher, not about this entity | `internal/infra/user_service_manual.go` |
| **The `PasswordHasher` port and its Argon2id adapter** | not a framework concept at all — the framework explicitly never sees a password (`token-issuance.html`) | port in `internal/domain/`, adapter in `internal/infra/` |
| **Both password operations** (§B Q2 / §E) | a custom command per `custom-command-handler.html`. Neither is a shape the generator has: the CHANGE is **routeless-by-id, public, and looks up by e-mail**; the RESET is gated on identity (`sub` == `:id` or `*:*`) rather than on a permission, and `authz.permissions` only maps operations to permission literals | `internal/application/commands/` + `internal/web/user_routes.go` |
| The `publicRoutes` entry for the open endpoint | a boot-configuration line, not code — `auth.publicRoutes` in **both** `microservice.dev.yaml` and `microservice.prd.yaml`. **Omitting it in prd is a boot failure** once `authorization.enabled` is on, which is the good direction | the two profile yamls |
| The **dummy-hash constant-time path** on an unknown e-mail (§E) | there is nothing to declare — it is a deliberate wasted Argon2id call, and no spec language has a word for "do useless work on purpose" | `internal/infra/` — the `PasswordHasher` adapter |
| ~~U1b's conditional tenant source~~ (§7) | **CLOSED at `omnicore-gen` 0.40.0** — `assignedFrom: identity-claim` + `bypassMaySet: true`: the server reads it off the caller's identity, and the caller who crosses the row scope states it instead. Reported upstream rather than routed around, and fixed | generated |
| **U7i — the lock check** | `rules.manual`. An ordinary probe, run inside the credential barrier | `internal/domain/user_rules_manual.go` |
| **U7h — the failure counter** | **NOT a rule at all, and the first draft was wrong to file it as one.** It persists on a request the domain REJECTED, and a rejected aggregate write persists nothing — so it cannot live in `BuildRules` on any path. It is the custom handler's failure branch issuing **its own** write before returning the 401 | the open change command handler |
| **U15** (archive forces `suspended`) — *listed above* | | |
| The three cross-aggregate foreign keys | a reference to ANOTHER aggregate is outside the spec language — the generator writes the parent key only | the `users` migration's `_manual.up.sql` |
| ~~The global partial unique index on `email`~~ | **CORRECTED — it IS generated.** `fields[].unique.within` is optional and names the fields uniqueness is scoped BY; omitting it gives a global index, and `scope: active-only` gives the partial predicate. The first draft listed this as hand-written from an assumption, not from the key documentation | generated |

**The read joins are NOT in that list.** `joins:` is a first-class key of the spec language,
so all three traversals of §2 are generated — declaration, entry-struct fields, DTOs and the
`?fields=` vocabulary — with nothing adopted and nothing hand-written.

### What stays hand-written EVEN AFTER the generator gains a body-fed field

*(Asked at the gate while the generator fix is being written. Answered against
`explain coverage` and `explain keys` at 0.39.0, not from memory. The `source: body` change
removes exactly one row from the list below — the three credential inputs — and nothing
else.)*

| Still hand-written | Why the generator cannot, verified | Is it a designed escape or a gap? |
|---|---|---|
| **The two credential operations, end to end** — command, handler, request and response shapes, route, tests | `authz.permissions` keys are a **closed set**: insert, update, patch, delete, archive, unarchive, read. There is no key for a custom operation, so there is no way to declare one at all — let alone one that is **public**, one identified by **e-mail instead of the row id**, and one whose gate is **three acceptors** (self OR bypass OR a permission) where the language maps an operation to exactly one literal | **a gap**, but a much bigger one than the field — this is "custom operations" as a feature, and `custom-command-handler` is the framework's own answer to it. Not worth asking for |
| **The password hasher port and its Argon2id adapter**, including the constant-cost path on a missing address | not a framework concept at all — the framework states it never sees a password | designed: it is ordinary consumer code |
| **`vos.Password`** | a raw VO's declarative rules are `required · length · range · comparison · requiredIf` — **there is no pattern kind**, and a four-character-class rule would not be one regex even if there were: Go's regexp engine has no lookahead, so "contains at least one of each class" is not a single pattern | designed: `written: manual` |
| **`vos.Email`** | same — a format check has no declarative kind | designed: `written: manual` |
| **`vos.PersonName`** (the composite) | its length bounds ARE declarable via the composite's own rule list; the anti-junk predicates are not | designed: `written: manual`, though the length halves could be declared |
| **The rules needing a cross-aggregate probe or the hasher** — U3, U6b, U7b, U7g, U7i, U8, U9, U12a/b, U13a/b | `rules.manual` is the designed hook for exactly this | designed |
| **U15** (archive forces `suspended`) | the rule DSL states refusals; this is an assignment | designed: `rules.manual`, and `Tenant` already ships the identical shape |
| **U7h** (the failure counter) | it persists on a **rejected** request. Not expressible anywhere, because a rejected write persists nothing — it belongs to the custom handler's failure branch | falls out of the custom-operation gap above |
| **The bodies of the nine service facts** | `service.facts[].kind: manual` with a declared `returns` — the **port method and its signature are generated**, only the body is yours | designed, and cheaper than it looks |
| ~~U1b's conditional owner source~~ | **CLOSED at 0.40.0** (`bypassMaySet: true`) | generated |
| **The two cross-aggregate foreign keys** | the generator writes the parent key only | designed: the migration hook |
| **The public-route declaration in both profiles** | boot configuration, not code | designed |
| **The mount order** between the public route and the by-id route | one is generated and one is not; ordering between them is a wiring concern | designed |

**What the `source: body` fix DOES buy**, so the change is not undersold: the entity struct,
the request DTOs, the commands and their mappers, the whole read side, the schema, the three
traversals, the migrations, the wiring, the seven catalogs and the generated tests all come
out of the generator with **nothing adopted** — including the create operation, which is the
one that carries the password. What is left by hand is the two credential operations and the
list above, all of it in hook files and in ordinary consumer code that a regeneration never
touches.

### Traps to check against the emitted code

- The cap notifications must carry their interpolated bound
  (`TooManyGroupsForUserNotification{Max: "50"}`, not an empty struct).
- `authz.dataAccess: tenant` must feed its write guard on **all four** per-entry child
  mappers as well as on the root's — otherwise a holder of `user:grant` can attach into
  another tenant's user.
- The root-archive auto handler must be instantiated **once per surface**, and there are now
  four child routes to wire it to by mistake instead of two.
- **`hidden` on `PasswordHash` must be verified on all four read paths** — by-id, listing
  row, write results and the export renderer — not assumed from the flag. This is the one
  check in this spec whose failure is a credential leak rather than a bug.
- **`redact` on `PasswordHash` must be verified on the emitted `TableSchema`**: the field is
  declared with `RedactedField(...)` and **both** axes present. A missing axis is a boot
  panic (loud, fine); a `core.Plain()` that slipped onto `InAudit` is silent, compiles, boots
  and writes every hash into `audit_events` and the `slog` echo. Grep the generated schema
  for `RedactedField("PasswordHash"` and read both arguments.
- **Prove it, do not infer it.** Insert a user on the bench, then read the `audit_events`
  row: the snapshot must carry `"PasswordHash": "***"` — the key **present** and masked,
  never absent. Then run the change-password operation and read the delta: one entry for
  `PasswordHash` with `"from": "***"` and `"to": "***"`. This belongs in the `/omnicore:qa`
  suite, because it is the only assertion here that a unit test structurally cannot make.
- **The `PasswordHasher` adapter must never log its input**, at any level, including on the
  error path. It is the last place a plaintext could still reach a log line after §2 removed
  every other one — `redact` cannot help, because the value never becomes a column.
- **U15 must be verified against the ROW, not only the audit event.** Archive a user, then
  `SELECT status, deleted_at FROM users WHERE id = …`: `deleted_at` set **and** status
  `suspended`. A rule that reached only the audit trail would look correct in the event and
  leave an archived-and-active row in the table.
- **U7g must be declared FIRST and must actually be a `guard`.** Test it the way an attacker
  would, not the way a user would: one request with an **unregistered e-mail** *and* a
  6-character new password must return **exactly one** notification. A second one in that
  response is the enumeration oracle §E exists to close, and it will pass every
  happy-path test ever written.
- **The unknown-e-mail path must cost the same as the known one.** Time both; if the miss
  answers in a millisecond, the dummy hash is missing. This is a wall-clock assertion, so it
  belongs to `/omnicore:qa`, not to a unit test.
- **The public route must be public in BOTH profiles**, and the prd one is the one that gets
  forgotten. `grep publicRoutes microservice.*.yaml` — two hits, same path, same method.
- **`PATCH /users/password` must be registered BEFORE `PATCH /users/:id`** (§E). Read the
  mount function and confirm the order, confirm the comment is on the line, and confirm the
  by-id route parses `:id` as a UUID. The test asserts the failure direction: a valid body to
  `/users/password` answering **401** means `/users/:id` swallowed it and the auth middleware
  answered first — which looks like a permissions bug and is a routing bug.
- **U7h writes on a FAILED request** — the counter must survive the refusal. Test it
  directly: five wrong passwords, then `SELECT failed_login_attempts, locked_until`. A
  transaction that rolls back on rejection would leave the counter at zero and the lockout
  would silently never engage, while every happy-path test stays green.
- **A locked account must answer the SAME generic 401**, not a distinct "account locked".
  Assert the response body is byte-identical to the wrong-password one.
- **`FailedLoginAttempts` and `LockedUntil` must be `hidden`** — never in a response, never
  in a filter. Publishing how close an account is to locking hands an attacker the counter.
- **`vos.PersonName` must NOT declare `Value()`** and must reach the schema through
  `Composite(...)`, never `Field(...)`. Both are boot panics that name their own fix, but the
  first one is the easy slip: every other VO in this project declares `Value()`.

### One constraint known to be unreachable by unit test

`internal/infra/` and the route-mount functions: exercising a repository, a fact or a route
needs a live relational engine and a running app, which is `/omnicore:qa`'s territory. This
is repo-wide, not specific to this entity, and it is what `CLAUDE.md` rule 6's 95% floor
collides with — the maintainer's to accept or to fund with a test harness.

**The Argon2id adapter is the exception and must be unit-tested to 100%**: it needs no
engine and no app, and a hashing bug that a test would have caught is not a bug anyone
notices until it is a breach.
