previous omnicore pin: v0.71.0
target: v0.72.0
build tags: postgres
restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go build -tags postgres ./...
