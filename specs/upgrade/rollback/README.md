previous omnicore pin: v0.65.0
target: v0.66.0
snapshot taken before bump
restore: cp specs/upgrade/rollback/go.mod go.mod && cp specs/upgrade/rollback/go.sum go.sum && go build -tags postgres ./...
