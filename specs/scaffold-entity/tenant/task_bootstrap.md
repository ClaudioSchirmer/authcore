# task — bootstrap

Model authority: `spec.md` §7, §9, §10.

## Read BEFORE generating (mandatory, at pin v0.54.0)

| Section | Why this layer needs it |
|---|---|
| `bootstrap.html` | the wiring contract, what a feature registers, and the order things are assembled in |
| `features.html` | the feature interface this entity implements, including the GraphQL one |
| `graphql.html` | the contract that brings the declared-but-unmounted surface up |
| `service-layout.html` | where the feature lives relative to the entity's other layers |

## What to build

The entity's feature, registered in the existing composition root. The root currently
declares no features and no translations at all — this entity is the first, and both slices
stop being empty with it. Translations become mandatory the moment the first feature exists,
so the seven catalogs from the application layer are registered here, not left for later.

Because `spec.md` §9 puts this entity on GraphQL, its feature also implements the GraphQL
feature contract. That is what mounts a surface the service has been declaring inertly since
it was scaffolded: the yaml block and the wiring were already there, and no feature had
claimed them.

**The domain Service must be constructed and passed through to every write handler.**
`spec.md` §7 ends with the entity requiring one, and a handler left without it fails at
runtime with a service-required notification that the build cannot catch. This is the single
easiest thing in the whole entity to wire three-quarters of the way.

The existing composition root carries comments describing itself as an empty shell and
naming what will change when the first aggregate arrives. Those comments are now wrong;
update them rather than leaving a file that describes a state the service has left.

## Acceptance

- The feature is registered, and the seven catalogs with it.
- The GraphQL surface mounts — it answers instead of returning not-found, which is what it
  did while no feature implemented the contract.
- Every write handler has the Service set. A grep for the entity declaring that it requires
  one must be matched by the feature constructing one and by every write handler receiving
  it.
- The composition root's own comments describe the service as it now is.
- `go build -tags postgres` and `go vet -tags postgres` clean.
