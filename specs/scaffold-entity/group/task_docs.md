# Task 8 — docs

No framework docs to read. This layer records what was built where the repository already
keeps its decisions.

## What to update

**`README.md`:**

- The capability table currently lists this entity as *not started — target model agreed*.
  Move it to done, in the same voice the other three use: what was built, from which spec,
  and the honest note that it has **not been booted against a real Postgres in this working
  tree**.
- Add its section beside Tenant, Permission and Role — the field table, and the things a
  reader will otherwise get wrong. At minimum: the entries answer role **ids**, not role
  names, and why; the collection verbs are a **pair, not a trio**; archive is **one-way**
  here too and the reason is one level up from Role's; **`group:grant` is a fifth verb** and
  why this entity takes the split Role declined; and **"which groups confer role X?" is a
  typed 400** on this posture.
- Add the API shape block, mirroring the three that are already there.
- Note the cross-tenant rule and its **single notification**, including why the message does
  not distinguish "another tenant" from "does not exist".
- The reserved-platform-tenant row already says `Role` depends on it. **It now also carries a
  second dependency:** no wildcard-bearing role can be attached to a group through the API,
  so the platform's own super-admin **group** has to be seeded by migration beside that
  tenant and that role. Say so there, not only in the spec.
- The badge line at the top of the README must name the pin this entity was generated at.
  Check it against `go.mod` before writing anything else in this file — a badge that
  disagrees with the pin is the one error a reader trusts.

**`specs/scaffold-entity/group/tasks.md`:** fill the deviations table — every place the built
tree differs from the approved model, with the reason. An empty table is a valid and good
result; an unfilled one is not.

## Acceptance check

- The capability table, the entity section, the API shape block and the platform-tenant note
  all updated.
- The deviations table filled or explicitly empty.
- No non-English text outside the seven translation catalogs (`CLAUDE.md` rule 3).
