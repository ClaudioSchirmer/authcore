# task_application — Claim

Model authority: [`spec.md`](spec.md) §2, §8, §9, §10. Layout, naming and granularity:
`service-layout.html`.

## Read BEFORE writing this layer

| What | Where |
|---|---|
| insert / update / patch, the field contract per handler kind | `auto-handlers.html` · `command-handler.html` |
| the read's Result and its mapping | `auto-query-handlers.html` · `custom-query-handler.html` |
| what one write touches end to end (SQL ↔ audit verb; PUT/PATCH share the verb) | `lifecycle-map.html` |
| the tenant filter in `ToCriteria`, the write guard, the super-admin bypass | `authz-seams.html` (Layer 2/3) |
| the layer's process and traps | `conventions/application.md` |

## What this layer must contain

**Commands** — insert, patch, archive. Their mappers move the six fields between DTO and
aggregate, converting the id at the boundary (the wire carries text, the domain carries an
id) and leaving the claim name **exactly as it arrived**: no trim, no case fold, no prefix
handling of any kind. The patch mapper touches only the three fields §8 leaves in the partial
body; the three immutable ones are not members of it at all, so there is no value to accept
and none to assign.

**Queries** — by-id and by-params, with the filter and sort vocabulary tabled in `spec.md`
§9, including the two fields that arrive across the read join into the owner.

**The tenant seam, in both directions and they are not the same mechanism:**

- the reads force the caller's tenant into the criteria, so a by-id read of another tenant's
  definition answers **404** — it does not exist for this caller, which leaks nothing about
  who else exists;
- the writes do **not** get their containment from that filter, because a write loads the row
  through the repository and the read filter never touches it. The guard in the domain is
  what refuses a foreign row;
- the `*:*` holder crosses both. A resource wildcard does not.
- **absent identity and absent claim are two states.** The first happens only under
  `auth.mode: disabled`, which the framework's own boot guard allows in dev only, and it
  stands down. The second refuses.

**The owner on the wire.** The insert body carries the tenant id, deliberately, for the two
reasons `spec.md` §10 gives: a `*:*` operator must be able to create a definition in a
customer's tenant, and on a dev bench with authorization off nothing else would fill it. The
guard already refuses a foreign value, so putting it on the wire costs no isolation. It is
absent from the patch body by §8.

**Translations** — every notification and every field label this entity introduces, in all
seven catalogs.

## Traps

- **A mapper that "helps" with the claim name.** See `tasks.md` trap 1. This layer is the
  second most likely place for it after the value object.
- **The patch mapper assigning an excluded field.** If it can assign it, the exclusion is
  cosmetic and the domain refusal becomes the only thing standing — which is the asymmetry
  `ACCESS_MATRIX.md` already records against two existing entities.
- **A tenant filter read as "unconstrained" when the claim is missing.** Fail closed.

## Acceptance

- The three commands and the two queries compile and are reachable from the web layer.
- The patch body carries exactly the three fields §8 names, and no more.
- A by-params read with no explicit tenant filter still produces a criteria carrying one.
- Round-tripping a prefixed name through insert → load → response returns it byte for byte.
- All seven catalogs carry every new key.
