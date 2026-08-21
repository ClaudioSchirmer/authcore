Rollback point for the omnicore upgrade of 2026-08-20.

Previous pin: v0.54.0
Target pin:   v0.55.0

go.mod and go.sum here are verbatim copies taken BEFORE the bump.

Both files are tracked in git and the working tree was clean when the snapshot was
taken, so `git checkout -- go.mod go.sum` is an equivalent restore. The copies here
make rollback work with or without git.

To roll back:
    cp specs/upgrade/rollback/go.mod specs/upgrade/rollback/go.sum .
    go build -tags postgres ./...

The build tag set is `postgres` alone: the engine comes from `relational.dialect` in
microservice.*.yaml, and neither profile declares a `transport:` block.
