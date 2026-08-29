# Backlog

Ideas and open questions that are **not** decided yet. Nothing here is a commitment: an
entry earns a spec (`specs/scaffold-entity/<entity>/spec.md`) and a README row only after
the maintainer approves it. Entries stay until they are either promoted or dropped with a
reason.

---

## Custom claims on `Group`

**Status:** open question — raised 2026-08-24, not approved, not specified.

Today a group carries only its identity (`key`, `name`, `description`) and its **role
bundle**: belonging to it grants a member every permission of every role attached to it.
The only thing a group contributes to a token is permissions, and the only non-permission
claim the platform mints is the scalar `tenant_id` (see README, *One e-mail, one user*).

We may need groups to carry **arbitrary tenant-defined claims** as well — key/value pairs
attached to the group and merged into the token of every member who belongs to it, so a
consuming service can branch on tenant-specific facts (department, cost center, region,
plan tier) without authcore learning that vocabulary.

Why it might be needed:

- Consumers keep asking authorization questions that are not permission questions.
  Modelling each one as a permission inflates the catalog with values that gate nothing.
- The group is already the natural place a tenant expresses "these people are alike" — the
  same edge that carries the role bundle would carry the attribute.

What has to be answered before this becomes a spec:

- **Merge semantics.** A user reaches several groups; two of them set the same key to
  different values. Union into a list, last-writer-wins, or refuse the configuration?
  Effective permissions today have *no precedence and no deny rule* — a claim map with
  precedence would be the first place that stops being true.
- **Claim-size budget.** The role bundle is already described as a budget in
  `specs/omnicore-gen/group.omnicore.yaml`. Free-form key/values on top of it push the JWT
  toward the header limits of every proxy in the path.
- **Namespace and reserved keys.** A tenant must not be able to set `tenant_id`, or any
  future platform claim, from a group. That needs a reserved prefix and a rule that
  refuses it, not documentation.
- **Direct grants.** Roles can be granted to a user directly, bypassing groups. Does the
  same apply to claims — a per-user claim map — or are groups the only carrier?
- **Type of the value.** String-only keeps the token predictable and the schema trivial;
  anything richer (numbers, lists, nested objects) is a JSON column and a validation
  surface.

Alternatives worth weighing against it:

- Leave attributes to the consuming service, keyed by `group.key`, and let authcore issue
  nothing but permissions.
- Put the claims on `Tenant` instead of `Group` — one map per tenant, no merge problem —
  if the real need is tenant-wide facts rather than per-cohort ones.

**The dependency is resolved as of 2026-08-26.** Token issuance is built: `POST /auth/user/token`
walks both arrows into one effective-permission set and signs it, so there IS now a minting
path and a claim map to merge into. This entry stops being blocked and starts being a
decision nobody has taken.

Three things the implementation settled, which sharpen the open questions rather than answer
them:

- **The claim-size budget is no longer hypothetical.** The shipped token deliberately carries
  group and role **keys** and not their display names, precisely because a token rides in a
  header on every request to every service and `description` is a `VARCHAR(500)` per group.
  Free-form tenant key/values would land on top of a budget that was already argued down to
  the minimum. Whatever merge rule wins, the size rule has to come with it.
- **The reserved-key problem now has a concrete list.** The platform mints `sub`, `tenant_id`,
  `tenant_workspace`, `email`, `name`, `permissions`, `groups`, `roles` and
  `must_change_password`. A tenant must be unable to set any of them from a group — and two of
  those (`permissions`, `tenant_id`) are read by the framework itself across the whole mesh,
  so overwriting one does not merely confuse a consumer, it changes what every service
  authorizes.
- **The audit allowlist is a second, separate decision.** `auth.auditClaims` controls which
  claims reach `audit_events.actorClaims`, and it was deliberately kept to two entries.
  Tenant-defined claims would need their own answer there: forwarding an arbitrary map into
  every audit row, one per write, forever, is not the same question as putting it in a token.

The precedence question is still the one that has to be answered first, and it is still the
one that breaks an existing property: effective permissions today have **no precedence and no
deny rule**, and a claim map with last-writer-wins would be the first place that stops being
true.

---

## Declined at the `Client` model gate (2026-08-26)

Two additions were weighed and turned down. They are here with the reason rather than as bare
ideas, so a future reader sees a decision instead of an omission.

**A hard expiry on a client secret** (`secretExpiresAt`). Real hygiene — it is what forces
rotation instead of letting a credential live for years — and also a scheduled production
outage the first time it fires. To be worth taking it needs a warning path: something has to
tell somebody thirty days out. This service has no outbound channel at all — no CDC relay, no
mail — so the feature would ship as a timer that silently breaks an integration at 03:00.
**Revisit when an outbound channel exists**, not before.

**Scope-down claims on a grant** — letting a role grant say "this client may act only on
tenant X's billing resources". It is the custom-claims question from the entry above wearing a
different hat, and it inherits the same unanswered problem: effective permissions today have
**no precedence and no deny rule**, and a scoped grant is a deny rule by another name. The
precedence question has to be answered first, and answering it for clients alone would mean
two authorization models in one service.

## Not started: `POST /auth/client/token`

The `Client` entity exists; the route that authenticates it does not. It is a capability
rather than an entity, so it belongs to `/omnicore:implement`, and the contract it has to
honour is already written: `specs/scaffold-entity/client/spec.md` §F lists the row shape it
reads, the eligibility checks, `identity_kind = 'client'` on every attempt row (which makes
the existing lockout apply unchanged), no refresh token per RFC 6749 §4.4.3, and the claim
set — `sub`, `tenant_id`, `tenant_workspace`, `name`, `identity_kind`, `permissions`, `roles`,
and no `email`, no `groups`, no `must_change_password`.

**Two prerequisites block it, and neither is this route's own work.** The source IP has to be
resolved correctly behind the proxy — `X-Forwarded-For` unguarded is spoofable, so an attacker
sets the header to an allowed range and walks through, while the socket IP alone is the load
balancer and blocks everybody. That is a trusted-proxy configuration and it is
`/omnicore:configure`'s territory. And `auth.auditClaims` has to gain `name` and
`identity_kind`, so an audit row says whether the actor was a person or a machine without
anybody cross-referencing the `clients` table.

**One thing to say out loud in whatever documents that route:** an allowed CIDR constrains
where a token is *obtained*, never where it is *used*. authcore does not see the requests a
client later makes to other services.

---

## Custom claims via a `Claim` catalog (tenant-owned, two levels)

**Status: PROMOTED — the catalog was approved and built on 2026-08-28.** The approved model is
`specs/scaffold-entity/claim/spec.md`; the generator spec is
`specs/omnicore-gen/claim.omnicore.yaml`; the entity is described in the README under
`### Claim` and its reach in `ACCESS_MATRIX.md` under `## Claim`. This entry stays rather than
being deleted, because most of what it asks is still open — what changed is that those
questions are now reachable instead of blocked.

**What the model gate settled, and what the answers cost:**

- **The reserved prefix is `x_`.** `ext_` was two runes dearer, a URI namespace (Auth0's
  answer) safest and by far the most expensive per token, and an explicit reserved LIST of the
  nine platform names cheapest of all — and refused, because it protects only against the names
  that exist NOW: the tenth platform claim would collide with a definition a customer already
  created, and there is no migration out of that.
- **The prefix is CALLER-OWNED, not server-owned.** The caller types it, the column stores it,
  a token would mint it, and nothing prepends or strips it anywhere. The alternative — a bare
  `cost_center` on the wire that the server prefixes — is this entry's own argument against a
  postiche internal name, wearing a prefix instead of a second column: the wire name and the
  token name would differ, so a consumer reading `x_cost_center` out of a JWT and searching the
  catalog for it would find nothing.
- **The reserved-platform-tenant dependency was BROKEN, not inherited.** This entry predicted
  the catalog would become the third entity blocked on that tenant. It is not: the prefix rule
  applies to every definition created through the API with no exception carved for a tenant, so
  the platform's own nine would enter by migration — the same door the `*:*` role enters by —
  and nothing in the entity needs to know which tenant is reserved.
- **Scope: the catalog only.** The two owned collections below are children of `User` and of
  `Client`, which already exist, so they are `/omnicore:evolve-entity` work — one run per
  parent — not this one. *(Both runs landed on 2026-08-28, plus a third
  on `Claim` itself for the narrowing hole they exposed.)*

**The two edge collections were built on 2026-08-28**, one run per parent —
`specs/evolve-entity/user/spec.md` and `specs/evolve-entity/client/spec.md`. Four of the
questions above are therefore answered, and they are recorded here as decisions rather than
struck out, because each one cost something:

- **Which verb sets a value: a new one.** `user:set-claim` and `client:set-claim`, not
  `user:grant` / `client:grant`. The criterion was the one this entry states — "riding
  `user:grant` means whoever may hand out roles may also set claims, and those are not
  obviously the same job" — settled by the precedent `ClientAllowedCIDR` had just set two days
  earlier with `client:manage-network`, and by one turn more: authcore cannot see what a
  consumer does with `x_cost_center`, so setting a value is potentially conferring privilege
  **in a way this service cannot audit**. Cost: two more literals nobody guesses from the
  pattern, taking that list from five to seven.
- **The correction verb is a PATCH carrying only `value`**, and getting there took two passes.
  The first pass mounted the generator's `change`, which emits a full-body PUT: it made the
  caller re-send `claimID` on every correction AND made the definition an entry points at
  mutable — this entry's own `role_permissions` argument turned back on it. The verb was
  dropped, the gap reported, and omnicore-gen 0.49.0 (on framework v0.63.0) added
  `children[].change` with `shape` and `patchExcludes`. The collection now declares
  `shape: patch` and `patchExcludes: [ClaimID]`: the definition is read off the stored entry,
  so there is no field in which to send a different one. Framework v0.63.0 sits underneath,
  refusing a child change that collides with another ACTIVE entry's identity (409) — the
  duplicate half, for every consumer.

- *(Superseded by the bullet above, kept because it is where the reasoning lives.)* **No
  `change` verb, decided on review the same day.** The entry was going to mount one, on
  the argument two paragraphs above — the identity is the `ClaimID`, `Value` is a separate
  mutable column, so a correction is one entry changing. The verb the generator emits does not
  honour that: it is a PUT whose body carries the whole entry, `claimID` included, so it makes
  the caller re-send what the server already knows AND makes the definition an entry points at
  mutable — which is this entry's own `role_permissions` argument turned back on it. The right
  shape is a PATCH carrying only `value`, and the spec language cannot say it (the root has
  `update.patchExcludes`, a collection entry has nothing). **Recorded as an omnicore-gen gap.**
  Correcting a value is archive + add meanwhile, which also keeps the old value readable on the
  archived row instead of overwriting it — `value` is one column with no history.

- **Removal on the edge: soft, like every other collection here.** The argument for a hard
  delete was "a claim value is not a privilege", and it falls with the verb decision above: if
  a consumer authorizes on `x_plan_tier`, an access review has to be able to read what a past
  value meant.
- **Type of the value: string, validated against the definition.** `valueType` is enforced at
  BOTH levels by one function, so the two cannot disagree about what a `bool` is.
- **A per-principal cap: 20.** A header budget before it is a count — 20 × 256 runes is ~5 KB
  of level-1 values riding on every request, which fits the 8 KB buffer most proxies default
  to. Fifty, the cap `groups` and `roles` carry, would be ~12.8 KB and would stop being a cap.

**One thing the build FOUND rather than inherited**, and it is closed too
(`specs/evolve-entity/claim/spec.md`): `appliesTo` was mutable in both directions, and
narrowing it while an edge held a value stranded that value invisibly — present, readable, and
un-writable. It is now refused for all four narrowing transitions; widening is untouched.

**What is still open**: the emission merge into `buildClaims`, the third-level question, the
audit allowlist, and the size budget the merge has to come with. Both levels of the chain can
now be filled, read and audited — **and no token has changed**, which is the same deliberate
line the catalog drew and the reason emission is a run of its own rather than a loose end.

The rest of this entry is the original draft, kept because it is where the reasoning lives.

Consumers need extra facts on a token that are neither permissions nor platform identity:
`cost_center`, `region`, `plan_tier`, `erp_id`. Two cases have to hold **at the same time**: a
**default** value shared by many accounts, and a **specialised** value that differs per
account. `Client` cannot borrow `Group` for this — it has no groups — and a single map on
`Tenant` mints the same claim into every token of both identity kinds, which is the case that
started the survey.

### What other systems do

- **Okta** — two named sources and an explicit fallback operator: `user.<x>` per account,
  `app.profile.<x>` per application, combined per claim with the Elvis operator
  (`user.costCenter ?: app.profile.costCenter`).
- **Auth0** — the same split under different names: `app_metadata` (per user) and
  `client_metadata` (per application); an Action merges them with `??`, checking the client id
  on the M2M path.
- **Cognito / Ory** — no declaration at all: a pre-token Lambda, or a Jsonnet mapper, does the
  whole merge in code.
- **AD FS** — the naming precedent: its registry is called `Claim Description` (claim type,
  name, description, publishing state). A catalog of CLAIMS, not of attributes.
- **Keycloak** — the cautionary one, and the closest to a "grant N bundles" design. It has a
  hardcoded-claim mapper (the default) and a user-attribute mapper (the specialised value) in
  the same pipeline; when two mappers target the SAME claim name only one value survives, and
  the priority order is neither documented nor stable (keycloak#25774, keycloak#16347). The
  practical advice that remains is "use different names".

The transversal lesson: **every system that sustains default + specialised has ONE ordered
place where precedence is written.** None lets precedence emerge from the binding — and the
one that came closest is the one with undefined behaviour.

### Why the registry is called `Claim` and not `Attribute`

The industry distinction is real but does not apply here. AD FS keeps the data in an
*attribute store* and a *Claim Description* names what leaves in the token; Keycloak keeps a
user *attribute* and a protocol mapper turns it into a *claim*. Both need two words because
the data exists independently of the token.

In this service it does not: the row exists to be minted. A registry entry would carry a `Key`
and a `ClaimName` that are always 1:1, with no transformation and no N:1 — two fields for one
value, which is what a postiche internal name looks like. Called `Claim`, they collapse into
one. The indirection only earns its keep alongside an `emitAsClaim` flag and an emission
policy, and neither is proposed here.

If a need ever appears for principal data that must NOT reach the token, it deserves its own
model rather than a boolean on this one.

### The chain — two levels

The claim name lives in the DEFINITION, never in the binding. One name is one definition, and
a principal holds at most one value per definition (a unique constraint on the edge).
Collision is therefore impossible by construction, and precedence is not a merge rule between
peers but an ordered chain for the same name — first non-null wins:

```
  user_claims.value  /  client_claims.value        level 1 — the specialised value
            ↓ if null
  claims.default_value                             level 2 — the default, of the
     (of the tenant that owns the definition)                tenant that owns it
            ↓ if null
  the claim does not enter the token at all
```

**Two levels rather than three, and `tenant_id` is why.** An earlier draft had a third level —
a `tenant_claims` collection holding a per-tenant default for a globally defined claim. Once
the definition itself belongs to a tenant, its `default_value` IS that tenant's default, and
the middle collection has no work left to do: one whole aggregate, with its verbs, its
permission and its seven translation catalogs, disappears.

What that trades away is recorded under the open questions: two tenants needing `region` write
two definitions, so the vocabulary can diverge between them.

### The registry — basic fields

Table `claims`. Tenant-owned, following the decision already taken for `Role`: *the platform's
own rows live in the reserved platform tenant rather than in a null scope*. **This entry
therefore inherits `Role`'s and `Group`'s dependency on that reserved tenant**, which the
README still lists as not started.

| Field | Type | Notes |
|---|---|---|
| `TenantID` | id | the owner, NOT NULL. The reserved platform tenant is what marks a row as the platform's — no `isPlatform` boolean is needed |
| `Name` | string | the exact name minted into the token. Unique PER TENANT over active rows |
| `ValueType` | enum | `string` · `number` · `bool` — what the edges are validated against |
| `AppliesTo` | enum | `user` · `client` · `both` — which identity kinds may hold a value |
| `DefaultValue` | string, nullable | level 2 of the chain; null means "no default" |
| `Description` | string | what the value means, for the operator filling it in |

Managed columns as everywhere else (`revision`, `created_at`, `updated_at`, `deleted_at`).

**Uniqueness is per tenant, and the reserved prefix is what keeps that safe.** Global
uniqueness would forbid two tenants from both naming a claim `region`, which is harmless — a
token carries exactly one tenant. But per-tenant uniqueness alone would let a tenant define
`permissions` or `tenant_id`. The prefix closes it by construction: a definition owned by any
tenant OTHER than the reserved one must carry it, and no platform claim does. The runtime
check is then a seatbelt rather than the mechanism.

### The two owned collections — basic fields

The SAME two fields on each parent, so the edge is one shape learned once:

| Field | Type | Notes |
|---|---|---|
| `ClaimID` | id | FK to `claims.id`; the entry's business identity |
| `Value` | string | validated against the definition's `ValueType`; never null on the edge |

| Parent | Child | Table | Parent column |
|---|---|---|---|
| `User` | `UserClaim` | `user_claims` | `user_id` |
| `Client` | `ClientClaim` | `client_claims` | `client_id` |

`businessIdentity: [ClaimID]`, unique per parent — that constraint IS the anti-collision
argument above, not a hygiene detail.

`editStrategy: per-child` with `operations: [add, remove, change]`. Note the difference from
`role_permissions`, which deliberately refuses `change`: there the single stored column IS the
business identity, so a change turns grant A into grant B while keeping A's row id. Here the
identity is the `ClaimID` and `Value` is a separate mutable column, so "correct this cost
center" is genuinely one entry changing rather than two events.

### Seeding the platform's own nine

Because the catalog is of CLAIMS, it can hold **all** of them — including the nine this
service already mints (`identity_kind`, `tenant_id`, `tenant_workspace`, `email`, `name`,
`permissions`, `groups`, `roles`, `must_change_password`), seeded by migration into the
reserved platform tenant. Worth doing for three reasons beyond tidiness:

- the catalog becomes the living documentation of the token's vocabulary, instead of a prose
  list that ages;
- `AppliesTo` then states as DATA what the `POST /auth/client/token` entry above states as
  text — that a client token carries no `email`, no `groups` and no `must_change_password`;
- two of those names (`permissions`, `tenant_id`) are read by the framework across the whole
  mesh, so having them present and platform-owned makes their reservation visible to anyone
  reading the table.

### Emission

`buildClaims` (`internal/application/commands/authentication_commands_manual.go:572`) keeps
its fixed set and merges the resolved map on top. A null at both levels means the claim is
simply absent — an absent claim and an empty one are not the same thing to a consumer. The
refresh path already rebuilds claims from the database on every redemption, so a corrected
value propagates on the next refresh with no special invalidation.

### What has to be answered before this becomes a spec

- **Does a tenant need to override the default of a PLATFORM-defined claim?** This is the one
  question that reopens the third level. If yes, `tenant_claims` comes back and the chain is
  three deep again. If no, the two levels above stand and the vocabulary simply lives per
  tenant.
- **The reserved prefix, literally.** `x_`? `ext_`? A URI, as Auth0 requires for
  collision-resistance? Whatever it is, it is the mechanism, so it has to be decided before
  the uniqueness rule can be written.
- **Who writes, and under which verb.** The catalog is tenant-owned, so a tenant admin creates
  definitions — but is setting a VALUE on a user the same job? A new `*:set-claim`, or the
  existing `*:grant`? The criterion is exact: riding `user:grant` means whoever may hand out
  roles may also set claims, and those are not obviously the same job.
- **Removal semantics on the edge.** `role_permissions` soft-removes because an access review
  has to read what a past grant meant. A claim value is not a privilege — is a hard delete
  right, or does the audit story want the same stamp?
- **`auth.auditClaims`.** Forwarding an arbitrary map into every audit row, one per write,
  forever, is a separate decision from putting it in a token. The allowlist is deliberately
  two entries today.
- **Claim-size budget.** The shipped token carries group and role KEYS rather than display
  names precisely because it rides in a header on every request. Whatever this adds lands on
  top of a budget already argued down to the minimum.

### Deliberately left out of this shape: the bundle

Granting claims in a PACKAGE — a `claim_sets` aggregate in `Role`'s shape — is the one
addition that brings a genuinely new problem. Two packages granted to the same principal can
carry the same definition with different values, and that is exactly the Keycloak case:
undefined precedence reaching the token. Resolving it needs a rule (grant order, a priority on
the set, or refusing the conflicting grant at bind time — only the last keeps the
indeterminacy away from the token). The two levels above already deliver "default plus
specialised" without it.

The earlier `Custom claims on Group` entry asks a related question through a different
carrier; this shape does not answer it, since a user reaches several groups and the collision
returns there. That entry therefore stays open on its own terms: the built catalog removes the
collision by construction only along the chain it defines — one name is one definition, and a
principal holds at most one value per definition — and a group carrier reintroduces exactly the
multiplicity that construction avoids.
