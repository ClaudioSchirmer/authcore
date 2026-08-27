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
