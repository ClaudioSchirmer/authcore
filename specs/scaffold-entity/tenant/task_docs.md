# task — docs refresh (not part of the entity)

Runs LAST, after the final verify is green. Scheduled here rather than earlier because the
README asserts capability, and the only moment that assertion becomes true is after the code
exists and passes.

## Why this task exists

`../../../README.md` predates this run and describes a tenant registry as **built** when no code
existed at all. `spec.md` §0 records that contradiction in full. Left alone, the README would
now be wrong in a second, worse way: wrong about a service that does exist.

## What to reconcile

Against `spec.md`, which is the approved model, and against the code as generated:

- the framework pin — the README names a version two releases behind what `../../../go.mod` now
  carries;
- the tenant field table — the handle's name, its bounds, its regex, the public key derived
  from it, the commercial status field, and the fact that the display name is not unique;
- what the JWT carries, which is the derived public key and never the primary key;
- the tenant business rules, which the README lists in a form that predates most of this
  run's decisions;
- the API shape, including the responses that v0.54.0 added — a stale-write refusal on every
  root write, and a not-found on archiving a row that is not there;
- the current-state table, which must say what is built and what is not, without rounding up;
- the verification command it records for building the service, which collides with the
  bootstrap directory and reports a file-naming error rather than a compile result.

The README's architecture posture section and its escape-hatch reasoning about splitting
credential from membership are still accurate and are not to be rewritten — they were right
before this run and remain right after it.

## Acceptance

- No statement in the README describes capability the repository does not have.
- The tenant section and `spec.md` agree on every field, bound and rule.
- The pointers to where decisions are written down resolve to files that exist.
