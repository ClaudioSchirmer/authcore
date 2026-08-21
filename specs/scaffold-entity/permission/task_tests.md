# task: tests — Permission

## Read first (mandatory, at execution time)

- Convention: `conventions/tests.md` — what to cover per layer and how coverage is measured.
- `rules-dsl` and `old-state` — to write the update-rule tests against the real mode
  dispatch and the real old-state semantics rather than against a guess.

## What must be covered

Target: **≥ 80% per generated file**, measured from a cover profile produced with the
cross-package coverage flag over the internal tree, then read per file. A bare package
percentage does not satisfy this and neither does a file credited only to its own package's
tests.

- **The composite value object**, which is where most of this entity's logic lives:
  - the segment rule in both directions — the accepted slug, the rejected leading and
    trailing hyphen, the rejected double hyphen, the length bounds at their edges, the
    repeated-rune guard, presence;
  - the resource accepting a colon-joined path and the action rejecting a colon;
  - the wildcard accepted as a whole part and rejected as a fragment;
  - a wildcard resource with a concrete action refused, and with a wildcard action accepted;
  - the rendered-length cap;
  - that each failure is reported under the failing part's own name;
  - **the rendering** — a normal pair, a hierarchical resource, a resource-wildcard pair and
    the total wildcard all render exactly as expected.
- **The aggregate's rules** — every branch: duplicate found and not found, on insert and on
  update with exclude-self; the key's immutability firing on a changed resource, on a
  changed action, and staying silent when both are unchanged; the old-state guard on insert
  (where the snapshot is nil); the description-echo rule.
- **The command mappers** — building the composite from two wire strings on insert;
  the lenient partial apply, including that a body carrying only the description leaves the
  key untouched.
- **The queries** — the criteria translation per declared filter, and **the computed
  derivation**: given the two part values on a result, the rendered field comes out right
  and matches what the value object itself produces, and it is left alone when a source is
  absent.
- **The responses** — that the projection carries the id, the description and the permission
  and does not carry the resource or the action.
- **The schema** — that the composite decomposes to the two expected columns under the two
  expected exposed names.

## Acceptance

- Every generated file's coverage line is reported; a file under the target is either
  brought up or surfaced as an explicit deviation for the maintainer to accept.
- No test is edited to make production code pass — the test is the oracle.
