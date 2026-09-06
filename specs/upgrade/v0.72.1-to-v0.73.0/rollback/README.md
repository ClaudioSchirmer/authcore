previous omnicore version: v0.72.1
target: v0.73.0
build tags: postgres
restore: cp specs/upgrade/v0.72.1-to-v0.73.0/rollback/go.mod go.mod && cp specs/upgrade/v0.72.1-to-v0.73.0/rollback/go.sum go.sum && go build -tags postgres ./...
