# Rollback snapshot — omnicore v0.67.1 → v0.68.0

Taken before the bump on 2026-08-31. Exact restore point for `go.mod` + `go.sum`.

previous version: v0.67.1
target version:   v0.68.0

NOTE: at snapshot time `go.mod`/`go.sum` were UNCOMMITTED at v0.67.1 (HEAD still carried
v0.67.0), so `git checkout go.mod go.sum` is NOT an equivalent restore — it would land on
v0.67.0. Use the file copies below.

restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go vet -tags postgres ./... && go build -tags postgres ./...
