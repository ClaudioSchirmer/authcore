# task — application

Model authority: `spec.md` §2, §7, §8, §9. Layout/naming/granularity: `service-layout.html`.

## Read BEFORE generating (mandatory, at pin v0.57.0)

| Section | Why this layer needs it |
|---|---|
| `auto-handlers.html` | the handler per verb, which mapper each one requires, and the strict-vs-lenient body contract that separates a full-body update from a partial one |
| `command-handler.html` | the insert/update pipeline and where the entity is built |
| `auto-query-handlers.html` | the read Result and how it is filled, plus the reserved read controls the request DTO opts into |
| `custom-query-handler.html` | the Result anatomy and the by-params read shape |
| `lifecycle-map.html` | what one write touches end to end — SQL, outbox, audit verb — so the actionName map is written against reality |
| `status-mapping.html` | the notification → HTTP mapping the results depend on |
| `service-layout.html` | where commands, queries, DTOs and catalogs live |

**Two facts this layer must carry honestly:** every root update is guarded on the revision
it was loaded with, so a stale write is refused with a concurrency notification rather than
silently overwriting — the update and archive results must report that outcome. And archive
and unarchive execute the update path, so their audit entries carry a changes block.

## What to build

**The insert command** — accepts the four caller-supplied fields of `spec.md` §9 and
**computes the public tenant id** in its entity mapper by calling the derivation method on
the workspace value object. That mapper is the only place in the system where the
derivation runs (`spec.md` §C.3): not in `BuildRules`, which is a validation pass that may
run more than once, and not in the handler, which runs after the rules window. The status
value arrives as a wire token and is converted through the framework's enum parser, so
anything unrecognized converges to the Unknown sentinel and is refused by validation rather
than persisted.

**The partial-update command** — the updatable set of `spec.md` §8 and nothing more. The
workspace and the public tenant id are absent by construction, which is what makes their
immutability structural rather than hopeful.

**The archive and unarchive commands** — bodyless verbs. Note that the status mutation of
rule 13 belongs to the domain's `IfArchive` closure, not to the archive command's mapper;
the command's job here is the verb, not the business rule.

**The queries** — by-id and by-params, with the criteria built from the filter vocabulary of
`spec.md` §9. Free-text search is deliberately not among them: the relational backing
answers it with a typed 400, and declaring it would promise what the posture cannot serve.

**The DTOs** for each command and query result.

**The actionName map**, so PUT and PATCH — which share an audit verb — stay distinguishable,
and so each operation's audit entry names the door it came through.

**Seven translation catalogs** covering: the twelve custom notifications of `spec.md` §7,
the three enum description keys of the status type, and the field labels of the five fields.
All seven languages carry real translations — this is the one place in the codebase where
non-English text is not only allowed but required.

## Acceptance

- The derivation runs in exactly one place, and that place is the insert command's entity
  mapper.
- The update command cannot express a change to the workspace or to the public tenant id.
- Every notification and every enum member the domain can emit resolves in all seven
  catalogs — a missing key renders as the key itself, which is a defect that no test
  catches unless the catalogs are checked as a set.
- The queries declare exactly the read controls of `spec.md` §9 — declared is served,
  undeclared is a typed 400, and that is a contract rather than an omission.
- Layout and naming match `service-layout.html`.
- `go build -tags postgres` and `go vet -tags postgres` clean.
