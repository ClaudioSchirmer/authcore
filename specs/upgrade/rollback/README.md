Rollback point for the omnicore upgrade of 2026-08-19.

Previous pin: v0.53.0
Target pin:   v0.54.0

go.mod and go.sum here are verbatim copies taken BEFORE the bump. They are the only
restore point: both files are untracked in git, so `git checkout` would not recover them.

To roll back:
    cp upgrade/rollback/go.mod upgrade/rollback/go.sum .
    go build -tags postgres ./...
