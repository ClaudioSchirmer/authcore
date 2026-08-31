# Rollback snapshot — omnicore v0.66.0 → v0.67.0

Taken before the bump on 2026-08-31. Exact restore point for `go.mod` + `go.sum`.

previous version: v0.66.0
target version:   v0.67.0

restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go vet -tags postgres ./... && go build -tags postgres -o /tmp/authcore-rollback ./bootstrap
