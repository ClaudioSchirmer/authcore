# Rollback snapshot — omnicore upgrade

Exact restore point taken before bumping the omnicore pin.

- Previous version: `v0.58.0`
- Target version:   `v0.59.0`
- Taken on:         2026-08-24
- Build tags:       `postgres` (both profiles declare `relational.dialect: postgres`
  and neither declares a `transport:` block)

`go.mod` and `go.sum` in this directory are verbatim copies of the repository root
files as they were at `v0.58.0`. They replace the snapshot of the previous run
(`v0.57.1` -> `v0.58.0`), which is superseded and no longer a valid restore point.

## To roll back

```sh
cp specs/upgrade/rollback/go.mod go.mod
cp specs/upgrade/rollback/go.sum go.sum
go build -tags postgres ./...
```

The same tag set as the upgrade's verify — an untagged build can be green while the
tagged one is not, which would "confirm" a rollback that isn't.

## Do NOT use `git checkout go.mod go.sum` for this snapshot

Unlike the previous run, `git status -- go.mod go.sum` was **not** clean when this
snapshot was taken: both files carried uncommitted edits from the in-flight
`feature/role-aggregate` work. The git path would silently destroy those edits.
The `cp` restore above is the only correct rollback for this run.
