# Rollback snapshot — omnicore upgrade

Exact restore point taken before bumping the omnicore pin.

- Previous version: `v0.55.0`
- Target version:   `v0.56.0`
- Taken on:         2026-08-21
- Build tags:       `postgres` (both profiles declare `relational.dialect: postgres`
  and neither declares a `transport:` block)

`go.mod` and `go.sum` in this directory are verbatim copies of the repository root
files as they were at `v0.55.0`.

## To roll back

```sh
cp specs/upgrade/rollback/go.mod go.mod
cp specs/upgrade/rollback/go.sum go.sum
go build -tags postgres ./...
```

Restoring the snapshot reverses `go get` **and** `go mod tidy` together; `go get
@v0.55.0` alone would not.
