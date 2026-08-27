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

# ── the dev signing key ──────────────────────────────────────────────────────
#
# Mirror of start.sh, and it must stay in step with it: this service is its own
# IdP, so a bench without JWT_SIGNING_KEY fails the boot at NewIssuer rather than
# starting insecurely. Generated once, never committed.
$KeyFile = 'devops/dev-signing-key.pem'
if (-not (Test-Path $KeyFile)) {
    Write-Host "start.ps1: generating a dev signing key at $KeyFile (not committed)"
    New-Item -ItemType Directory -Force -Path (Split-Path $KeyFile) | Out-Null
    # PKCS#8, RSA 2048 — the Issuer rejects RSA under 2048 bits at construction,
    # and parses PKCS#8 specifically.
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out $KeyFile
}

# THE NEWLINES MUST BE LITERAL BACKSLASH-N. Config interpolation runs on the raw
# file text before the yaml is parsed, so a PEM with real newlines substituted into
# `privateKeyPem: "${JWT_SIGNING_KEY}"` yields a double-quoted scalar broken across
# lines — yaml folds those newlines into spaces and the key silently fails to
# parse. Two-character `\n` survives both passes.
$env:JWT_SIGNING_KEY = ((Get-Content $KeyFile) -join '\n') + '\n'

# Ties the token's `kid` to the key material, so a stale token stops validating
# instead of failing obscurely against a key that no longer matches.
$Digest = (Get-FileHash -Algorithm SHA256 $KeyFile).Hash.ToLower()
$env:JWT_SIGNING_KID = "dev-$($Digest.Substring(0,8))"

docker compose -f devops/docker-compose.yml up -d --wait
go build -tags 'postgres' -o .\bin\authcore.exe .\bootstrap
$env:APP_PROFILE = 'dev'
.\bin\authcore.exe
