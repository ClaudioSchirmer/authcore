# Rollback point — omnicore v0.69.0

Taken before bumping `github.com/ClaudioSchirmer/omnicore` from **v0.69.0** to **v0.70.0**.

`go.mod` and `go.sum` here are verbatim copies of the files as they stood at v0.69.0.

## To restore

```sh
cp specs/upgrade/rollback/go.mod go.mod
cp specs/upgrade/rollback/go.sum go.sum
go build -tags postgres ./...
```

The build tag set is `postgres` (no `transport:` block in either profile) — the same
set the upgrade was verified with. An untagged build can be green while the tagged one
is not, so always restore with the tag.

Equivalent via git (`go.mod`/`go.sum` were clean at snapshot time):

```sh
git checkout go.mod go.sum
```
