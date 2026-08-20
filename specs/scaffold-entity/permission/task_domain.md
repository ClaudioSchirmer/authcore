# task: domain — Permission

## Read first (mandatory, at execution time)

- `value-objects` — the raw kind, and **the composite kind**: what it must declare
  (`IsValid`), what it must NOT declare (`Value()`), how a part that is itself a value
  object is validated from inside it, and where a part's label comes from.
- `rules-dsl` — the mode gates and how a notification is emitted.
- `old-state` — `domain.Old(e)` for the immutability rule, and the fact that it is nil on
  insert.
- `status-mapping` — which notification lands on which HTTP status (409 for the duplicate,
  422 for the rest).
- `table-schema`, "Supported column shapes" — only to confirm the pin's identity contract
  and the closed persistable set each composite part must draw from.
- Convention: `conventions/domain.md`. Layout and naming: `service-layout`.

## Model decisions that touch this layer

From `spec.md` §2, §5, §7:

- **One new value object: the permission key — the composite kind.** Two exported parts,
  both plain strings, each carrying its own label tag. It owns **both halves of the
  concept**:
  - **validating.** A private segment helper defines the shape once — a 2–64 rune lowercase
    slug, single hyphens, never leading or trailing, no run of 4 identical runes, reusing
    the anti-junk helpers the project already extracted. The resource part accepts a
    colon-joined path of segments; the action part accepts exactly one segment. Either part
    may be the wildcard, and only as its entire value. Each failure is emitted under the
    failing part's own name, with the two parts keeping distinct notifications.
  - **rendering.** `String()` returns the resource, a colon, and the action. This is the
    only place in the service where that separator appears. It must NOT be called `Value()`
    — that method is the discriminator the framework uses to tell a scalar value object from
    a composite, and declaring it here is a boot panic.
  - the cross-part rules only it can see: a wildcard resource forces a wildcard action, and
    the rendered token's length cap.

- **Reuse, do not copy:** the existing description value object is used as-is, and the
  anti-junk predicates the project already extracted as pure helpers are reused rather than
  re-derived. **No separate value object is created for the resource or for the action** —
  see `spec.md` §B Q6; the rule has one home and it is the composite.
- **The aggregate**: three persisted fields — the composite key and the description — each
  with a label tag and **nothing else on the tag**. It embeds the plain base entity, not the
  aggregate root: there is no child collection.
- **Modes**: display, insert, update, archive. **No unarchive and no delete** — see
  `spec.md` §B Q5; do not emit an `IfUnarchive` clause, and do not add the mode "for
  symmetry".
- **The service is required** — the aggregate declares it, because the duplicate pre-check
  needs an existence probe. Declaring it obliges every write path to receive one.
- **Rules** in `BuildRules`: the active-scope duplicate pre-check on the pair
  (insert-or-update, exclude-self), the key's immutability (update only, guarded on the
  old-state snapshot being non-nil), and the description-echoes-the-key check
  (insert-or-update). Nothing that a value object already validates is repeated here.
- **Notifications**: four from the value object (a malformed resource, a malformed action,
  the unmatchable wildcard pair, the over-long rendered token) in the value-object package's
  existing registration site; three from the aggregate (the duplicate, the frozen key, the
  echoed description) in the domain package's. Each struct name is the translation key, so
  it must match what the seven catalogs will carry.
- **A domain-side service contract** the infra implementation satisfies: an active-scope
  "is this pair taken, excluding this id" probe.

## Acceptance

- The composite declares `IsValid` and does not declare `Value()`; neither it nor its parts
  implement a JSON marshaler, and no part is tagged to be skipped by JSON.
- **The colon separator appears in exactly one expression in the whole service**, inside the
  composite's rendering method. Nothing else joins a resource to an action.
- No `json:` or `db:` tag anywhere in the domain package.
- No regex, length or membership check inside the aggregate's `BuildRules` — those live in
  the value objects.
- `Modes()` carries archive and not unarchive.
- Builds and vets clean with the postgres tag.
