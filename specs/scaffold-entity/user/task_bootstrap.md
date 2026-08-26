# task_bootstrap.md — bootstrap

Model authority: [`spec.md`](spec.md) §9, §10, §E.

## What this layer must contain

**A feature for this entity**, registered in the composition root beside the four that
already exist, and following their shape exactly.

It must contribute:

- the repository and the read model — the latter through the **relational** feature seam, the
  sibling of the Mongo one, which is what this posture uses;
- the **domain service**, constructed and passed to the mount. This entity's domain declares
  that it requires one, so every write handler must receive it — a nil there is a runtime
  failure the build cannot catch;
- the **password hasher**, constructed once at boot and injected into the two credential
  commands. Its constant-cost fixture is generated at boot, not per request;
- the route mounts for both surfaces.

## The two configuration changes this entity needs

Neither is code, and both are easy to forget:

1. **The public route must be declared in the authentication bypass list of BOTH profiles.**
   The production one is the one that gets forgotten, and once the declarative authorization
   layer is switched on, a non-public route with no permission fails the boot scan — which is
   the good failure direction, and is also the check that catches the omission.
2. **The new permission literals** — the entity's own verbs, the grant verb and the reset
   verb — have **no catalog row until an operator inserts one**. Nothing here seeds them, on
   purpose: a migration that invents grants is a migration that grants power nobody reviewed.
   Say so in the report and in the README row rather than leaving it to be discovered.

## What to read before writing — routed sections at the pin

| For | Read |
|---|---|
| the feature contract and how a feature contributes each seam | `bootstrap` · `features` |
| the relational read-model seam | `relational-view` |
| the exact shape of the bypass declaration and the authorization block | `yaml-reference` · `auth-middleware` |
| file layout and naming | `service-layout` |

Convention: `conventions/bootstrap.md`.

## Traps specific to this layer

- **A required service that is not wired is a runtime failure, not a compile error.** Every
  write handler for this entity must receive it, including the four child ones and the two
  credential ones.
- **The read model is contributed through the relational seam**, not the projected one. This
  service has no Mongo, no broker and no relay; a projected view would never materialize.
- **Registration order matters for the two routes that share a shape** — see
  [`task_web.md`](task_web.md).

## Acceptance check

- The service boots against the local bench with the new feature registered.
- The migrations apply at boot in the development profile.
- The generated documentation page lists all eleven REST operations, and the GraphQL schema
  the five root ones.
- The public route answers without a token in the development profile.
- Both profiles declare the bypass; the production one is checked explicitly rather than
  assumed.
