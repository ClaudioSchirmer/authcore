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

Depends on **token issuance**, which is still *not started* (README, roadmap table). There
is no minting path to add a claim to yet, so this cannot be built before that exists.

**Half of that dependency resolved on 2026-08-26**: `User` is built, so the SUBJECT a token
would be minted for now exists, and both arrows into `Role` — through a group and directly —
are stored and served. What is still missing is the walk that turns them into one set of
effective permissions, and the `Issuer` call that carries it. Until those land, this entry
stays where it is: a claim map has nothing to be merged into.
