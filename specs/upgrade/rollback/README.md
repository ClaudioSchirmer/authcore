previous omnicore pin: v0.61.0
target: v0.61.1
snapshot taken before: go get + go mod tidy
restore: cp specs/upgrade/rollback/go.{mod,sum} . && go build -tags postgres ./...
