# task — domain

Model authority: `spec.md` §1, §2, §5, §7. Layout/naming/granularity: `service-layout.html`.

## Read BEFORE generating (mandatory, at pin v0.57.0)

| Section | Why this layer needs it |
|---|---|
| `value-objects.html` | the raw kind vs the enum kind, what the automatic pass discovers and validates, where the field label lives, and the closed persistable set a VO's underlying type must belong to |
| `rules-dsl.html` | the mode gates (`IfInsert` / `IfUpdate` / `IfInsertOrUpdate` / `IfArchive` / `IfUnarchive`), how a rule emits its notification, and how the domain Service is reached from inside a rule |
| `old-state.html` | what `domain.Old(e)` guarantees — the snapshot is captured when the entity is born, uniformly across all five state-changing verbs. The immutability guard depends on this |
| `status-mapping.html` | which notification maps to which HTTP status, so the spec's 422/409 column is honored rather than assumed |
| `service-layout.html` | where each type lives and how files are split |

**The fact this layer leans on:** a mutation performed inside an `IfArchive` closure reaches
the database — archive executes the update path, not a `deleted_at`-only statement. Rule 13
depends entirely on it. And it is `IfArchive` / `IfUnarchive` that fire on the archive verbs,
never `ModeUpdate`.

## What to build

**Shared text predicates** — the anti-junk helpers, pure functions over runes, with no
knowledge of any entity: run-of-N-identical, distinct-count, word-count, has-a-vowel,
trimmed-and-single-spaced. Their exact Unicode semantics are pinned in `spec.md` §7 ("How
the text predicates are defined") and are binding: runes not bytes, `unicode.IsLetter` for
letters and words, and the Unicode vowel definition that admits any non-Latin letter. These
are the single most reused thing this run produces — `Group`, `Role` and every later entity
compose them.

**Value objects**, per `spec.md` §2:

- a shared display-name type over `string` (raw kind) — bounds 2–120;
- a shared description type over `string` (raw kind) — bounds 15–500;
- a tenant-specific workspace type over `string` (raw kind) — bounds 3–63, the DNS-label
  shape, the reserved list, **and the method that derives the public tenant id** (UUIDv5
  over the namespace constant recorded in `spec.md` §C);
- a tenant-specific status type over `string` (enum kind) — three members plus the zero
  Unknown sentinel, declaring its members, its underlying value and its unknown
  notification, and writing **no** `IsValid`.

The shared types are shared deliberately (`spec.md` §2, "VO scope"); the line drawn there —
display name is for things, not people — is part of the decision and belongs in a comment
where the next entity's author will read it.

**The aggregate root**, carrying the five fields of `spec.md` §2 with their label keys, its
mode set (`spec.md` §5), and `RequiresService` answering true.

**`BuildRules`** — exactly the rules of `spec.md` §7 whose scope column is a mode gate.
The rules whose scope reads "auto (VO)" are NOT written here: the framework discovers and
runs them off the field types, and restating them makes one empty field produce two
complaints. The uniqueness pre-check reaches the domain Service; the immutability guard
reaches `domain.Old`; the status transition guard reaches `domain.Old`; rule 13 mutates
rather than validates.

**Notifications** — one type per custom notification named in `spec.md` §7, each carrying
the key its seven catalogs will resolve. Enumerated in the spec; there are twelve, and three
more come from the framework and are not redeclared.

## Acceptance

- Every rule of `spec.md` §7 is either in `BuildRules` under the scope the spec names, or
  in a value object because the spec's scope column says "auto (VO)" — and nothing appears
  in both places.
- No `json:` and no `db:` struct tag anywhere in the domain: a domain field carries its
  label key and nothing else.
- No format, regex, length or range check sits inline in `BuildRules` — those live in value
  objects, which is where their tests live too.
- The mode set and the archive column declaration agree (the schema layer is where the
  second half of that agreement lands, but the mode set is decided here).
- Layout and naming match `service-layout.html`.
- `go build -tags postgres` and `go vet -tags postgres` clean.
