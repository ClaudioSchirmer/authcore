previous omnicore version: v0.70.0
target: v0.71.0
build tags: postgres
snapshot taken before: go get + go mod tidy
restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go build -tags postgres ./...
