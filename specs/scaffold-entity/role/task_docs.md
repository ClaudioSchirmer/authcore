# task: docs — Role

Model: `spec.md`. Follows the precedent the two existing entities set: the project README
carries a section per aggregate, and the status table at the top is kept honest.

## What to write

**A README section for this aggregate**, in the same register as the two that exist: the
field table, then the small number of things a reader would otherwise get wrong. For this
entity those are:

1. **The read returns the permission, not just its id** — `resource`, `action` and
   `archivedAt` come across the foreign key at load time while the row still stores only the
   id. Say why the id is what is stored (a retired permission returns as a NEW row, so a
   stored string would silently re-attach), and say what `archivedAt` is for: a grant
   pointing at a retired permission is a normal long-lived state, and this is how a reader
   sees it.
2. **You can only grant what you hold**, and a superadmin is exempt by construction rather
   than by a special case.
3. **No wildcard can be granted through the API** — and where the platform's own wildcard
   role is therefore meant to come from.
4. **Archive is one-way here too**, for the same reason it is one-way on the catalog: a
   restore would silently re-authorize everyone still holding the role.
5. **Filtering by a granted permission is not available** — the join renders the
   counterpart, it does not make it addressable — and what would make it available.

**Update the status table** at the top of the README so this aggregate is no longer listed
as not started, and leave the reserved platform tenant's row honest — this run does not
create it, and §7 now has a stated dependency on it.

**Update the domain-model section's scoping table** only if it needs it; the row for this
aggregate already says what this build implements, which is the point of having read it
first.

## Acceptance

- Every claim in the new section is true of the code that was just generated, checked rather
  than copied from the spec.
- The status table matches the repository.
- English throughout, per rule 3.
