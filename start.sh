#!/usr/bin/env bash
# Bring up the local bench and run authcore in dev mode (always the dev profile).
# Build + exec, never `go run`: exec makes THIS pid the app, so SIGTERM reaches it
# and the drain runs. Under `go run` the signal stops at a parent that ignores it.
#
# The build carries the `postgres` engine tag and no transport tag: the yaml declares
# no `transport:` block, so the no-op transport is the correct adapter.
set -euo pipefail
cd "$(dirname "$0")"

docker compose -f devops/docker-compose.yml up -d --wait
go build -tags 'postgres' -o ./bin/authcore ./bootstrap
APP_PROFILE=dev exec ./bin/authcore
