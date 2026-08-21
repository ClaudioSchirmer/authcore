# Rollback snapshot — omnicore upgrade

Exact restore point taken before bumping the omnicore pin.

- Previous version: `v0.56.0`
- Target version:   `v0.56.1`
- Taken on:         2026-08-21
- Build tags:       `postgres` (both profiles declare `relational.dialect: postgres`
  and neither declares a `transport:` block)

`go.mod` and `go.sum` in this directory are verbatim copies of the repository root
files as they were at `v0.56.0`.

## To roll back

```sh
cp specs/upgrade/rollback/go.mod go.mod
cp specs/upgrade/rollback/go.sum go.sum
go build -tags postgres ./...
```

Restoring the snapshot reverses `go get` **and** `go mod tidy` together; `go get
@v0.56.0` alone would not.

## Earlier snapshots

This directory holds only the most recent restore point. The snapshot of the
previous upgrade (`v0.55.0` → `v0.56.0`) is still reachable through git history:

```sh
git show HEAD:specs/upgrade/rollback/go.mod
git show HEAD:specs/upgrade/rollback/go.sum
```
