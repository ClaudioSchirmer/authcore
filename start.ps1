#!/usr/bin/env pwsh
# Bring up the local bench and run authcore in dev mode (always the dev profile).
# Build + run the binary, never `go run`: the signal must reach the app itself.
# The build carries the `postgres` engine tag and no transport tag, because the yaml
# declares no `transport:` block.
#
# The default execution policy may block a bare .\start.ps1 — invoke it as
#   pwsh -File .\start.ps1
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

docker compose -f devops/docker-compose.yml up -d --wait
go build -tags 'postgres' -o .\bin\authcore.exe .\bootstrap
$env:APP_PROFILE = 'dev'
.\bin\authcore.exe
