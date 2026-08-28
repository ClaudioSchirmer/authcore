# task_domain — Claim

Model authority: [`spec.md`](spec.md) §1, §2, §5, §6, §7. Layout, naming and granularity:
`service-layout.html` — normative, and it outranks any mechanical detail written here.

## Read BEFORE writing this layer (mandatory at execution time)

| What | Where |
|---|---|
| rules, notifications, `Old()`, `actionName` | `rules-dsl.html` · `old-state.html` · `status-mapping.html` |
| value objects: raw vs enum, auto-validation, the unknown-member answer | `value-objects.html` |
| which LAYER declares a notification | `shared/notification-bases.md` (owner) · `status-mapping.html` |
| the domain service — WHICH primitive answers a rule's question | `shared/query-primitives.md` (owner) · `custom-command-handler.html` |
| file layout / naming / granularity | `service-layout.html` |
| the layer's process, decisions and traps | `conventions/domain.md` |

Local flavor to mirror (read, do not copy): the existing tenant-owned aggregate and its
hand-written value objects and manual rules.

## What this layer must contain

**The aggregate** — a flat root, six fields, modes `display, insert, update, archive`. No
collection: it declares no aggregate value object and no child anything. `RequiresService()`
is **true**.

**Three new value objects**, all under the project's `vos` package:

1. **The claim name** — a raw value object over `string`. Lowercase `[a-z][a-z0-9_]*`,
   2..64 runes, snake_case, plus this project's shared anti-junk predicates (the
   distinct-rune floor and the cap on identical runs already used by the role and group
   handles — **reuse the shared predicates, do not restate them**), plus the reserved
   prefix rule:
   - the value MUST begin with `x_`;
   - the remainder MUST NOT itself begin with `x_`;
   - **nothing is normalized.** No trimming, no lowercasing, no prepending, no stripping. A
     value that does not already comply is refused. This is the gate's decision (`spec.md`
     §2, OPEN-2 → B) and it is the single most reversible-looking thing in this layer: a
     one-line "convenience" prepend silently changes what every future token carries.
   - one notification for the whole shape, following the local precedent that a value object
     reports at most one "this is not valid" however many shape rules failed — emptiness
     stays the framework's own required-field answer, which is exactly why the aggregate
     declares no `required` rule on top of it.
2. **The value type** — an enum value object with members `string`, `number`, `bool` and an
   unknown sentinel that is deliberately absent from the member list.
3. **The applies-to** — an enum value object with members `user`, `client`, `both`, same
   shape.

`Description` is **reused** from the existing shared value object; nothing new is written
for it. `TenantID` is an id and validates itself.

**The rules**, exactly as `spec.md` §7 tables them: R1 (owner validated first, and it is the
BARRIER — everything after it depends on the owner being a usable id, most sharply the
uniqueness pre-check, which is scoped by it), R2/R3/R4 (immutability of name, owner and value
type), R5 (uniqueness per tenant over active rows, exclude-self on update), R6 (the owner
must exist and be available — a trial tenant passes), R7 (the default parses as the declared
type; a null default skips the check), R8 (the default's length).

R6 and R7 are **named manual rules** with descriptions — the generator cannot express either,
and an entry without a description is refused as an empty TODO. R7's whole content is in
`spec.md` §7; the implementer writes the parse, not a new decision.

**The notifications** — the ten listed in `spec.md` §7, each in all seven catalogs
(`../../../CLAUDE.md` rule 3). The two tenant ones the framework already owns and already
translates are **not** declared here.

**The service port** — two facts, each named for the PROBLEM rather than for the healthy
state, because the generated suite stubs the service so every probe answers "nothing found":
one exists-probe for "another active definition in this tenant already holds this name"
(exclude-self, active-only), and one manual probe for "the owning tenant is missing, archived
or commercially suspended". No caller-identity fact: this aggregate confers nothing.

## Traps this layer can actually trip

- **A `required` rule beside a value-object-backed field** tells the caller the same thing
  twice; `check` warns about it by name. Four fields here are value-object-backed.
- **A `json:` or `db:` tag on a domain field.** A domain field carries `labelKey` and nothing
  else; a `json:"-"` also corrupts the `Old()` snapshot.
- **`Old()` is nil on insert** — every immutability rule reads the pre-write snapshot, and
  the framework scopes those to update for that reason. Do not hand-roll a comparison that
  dereferences it.
- **Naming a fact for the healthy state** turns a correct spec red on the day it is written.
- **Reaching for `Role`'s escalation vocabulary.** There is none here, and adding it would
  invent a privilege model this aggregate does not have.

## Acceptance

- The aggregate, the three new value objects and the reused one compile; `go vet` clean.
- Every rule in `spec.md` §7 has a counterpart, and no rule exists that §7 does not list.
- The ten notifications exist with all seven catalogs filled — no placeholder, no English
  standing in for a missing translation.
- The name value object refuses, with evidence: an unprefixed name, a double-prefixed name,
  an uppercase name, a leading/trailing-space name, a name of one rune, a name of 65 runes,
  keyboard junk — and **accepts an ordinary prefixed name unchanged**, byte for byte.
- `grep 'json:"' internal/domain/` hits nothing new.
