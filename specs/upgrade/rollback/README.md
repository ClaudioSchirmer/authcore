# Rollback snapshot — omnicore upgrade

Exact restore point taken before bumping the omnicore pin.

- Previous version: `v0.56.1`
- Target version:   `v0.57.0`
- Taken on:         2026-08-23
- Build tags:       `postgres` (both profiles declare `relational.dialect: postgres`
  and neither declares a `transport:` block)

`go.mod` and `go.sum` in this directory are verbatim copies of the repository root
files as they were at `v0.56.1`.

## To roll back

```sh
cp specs/upgrade/rollback/go.mod go.mod
cp specs/upgrade/rollback/go.sum go.sum
go build -tags postgres ./...
```

The same tag set as the upgrade's verify — an untagged build can be green while the
tagged one is not, which would "confirm" a rollback that isn't.

The project is git-tracked, so `git checkout go.mod go.sum` is an equivalent restore
(`git status -- go.mod go.sum` was clean when this snapshot was taken, so nothing
uncommitted would be destroyed by that path). The snapshot exists so rollback works
with or without git.
