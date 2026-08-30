previous omnicore pin: v0.64.0
target: v0.65.0
snapshot taken before bump
restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go build -tags postgres ./...
