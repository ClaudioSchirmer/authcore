# task: application — Role

Model: `spec.md` §3, §7, §8, §9, §10. Convention: `conventions/application.md` +
`conventions/aggregate-children.md`. Layout/naming: `service-layout.html`.

## READ before writing (mandatory)

- `auto-handlers.html` — the six root verbs, the strict full-body vs lenient partial
  contract, and which mapper each handler calls.
- `command-handler.html` + `custom-command-handler.html` — where a command may read the
  request context, and the ctx-bound service.
- `auto-query-handlers.html` — the read's result shape and the criteria contract.
- `authz-seams.html` Layer 2 and Layer 3 — the command mapper is the ONLY layer allowed to
  translate the identity into business-named entity fields, and the query's criteria hook is
  where the tenant filter is injected.
- `query-side.html` — the paged read contract.
- `lifecycle-map.html` — what one write touches end to end, so the audit verb is understood
  before the child ops are written.

## What to build

**Root write operations:** a complete insert of the whole aggregate (root fields plus the
initial grant collection), a root patch, and an archive. Per §8 the patch is the only update
shape; only the display name and the description are actually reachable through it, because
the key and the owning tenant are frozen by rule and the grant collection moves through its
own operations.

**Two child operations, both commands ON THE ROOT** (load root → the domain method mutates
the one child → the framework persists the diff): grant one permission, and revoke one by
child id. There is no child update, because the child has no editable field. The child id
travels beside the input on the command, bound from an extra path segment — never inside the
input body.

**A child input type** for the grant, context-free and carrying field values only — the
catalog reference, and **only** that. The three join fields are read-side: they belong to the
child's row-result and to the response, never to an input, a command or a filter.

**The identity translation.** Every write command's mapper populates the two runtime-only
fields from the request identity: the requesting tenant, and whether the caller is a
superadmin. This is the one place `ctx` may be read on the write side. On insert the mapper
also stamps the owning tenant from the claim when the caller is not a superadmin, so a
tenant cannot create a role somewhere else by sending a different value.

**Both reads**, by id and by params, with the tenant filter injected into the criteria per
§10 — and skipped for a superadmin. The filter is written onto the criteria as the entity's
own field path; it passes no wire gate, so it is not a declared filter and cannot be
overridden by a caller.

**Fail closed on an absent tenant claim.** The service-wide switch is off, so the claim can
be empty. Empty must not mean "no filter" on the read, and must not mean "no check" on the
write.

**The seven translation catalogs** gain every notification introduced in §7 — real
translations in all seven, never the conversation language copied across. The two
tenant-isolation notifications already ship translated by the framework and are not
redeclared here.

## Acceptance

- No handler reads `ctx` outside a command mapper or the query's criteria hook.
- The grant and revoke commands mount their own handlers; neither touches the root archive
  handler (trap 1).
- Every new notification key resolves in all seven catalogs.
- The insert response mirrors the post-write aggregate WITH the minted child ids. Note what
  the join fields read on that response: an entry this write just added was never loaded
  through the traversal, so `Resource` and `Action` come back empty and `ArchivedAt` `null`.
  That is the framework's contract, not a defect — a client that needs them re-reads. Do not
  paper over it by having the mapper fill them.
- Builds and vets clean.
