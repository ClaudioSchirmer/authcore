#!/usr/bin/env bash
# Bring up the local bench and run authcore in dev mode (always the dev profile).
# Build + exec, never `go run`: exec makes THIS pid the app, so SIGTERM reaches it
# and the drain runs. Under `go run` the signal stops at a parent that ignores it.
#
# The build carries the `postgres` engine tag and no transport tag: the yaml declares
# no `transport:` block, so the no-op transport is the correct adapter.
set -euo pipefail
cd "$(dirname "$0")"

# ── the dev signing key ──────────────────────────────────────────────────────
#
# This service is its own IdP: auth.issuer signs with this key and auth.jwt
# validates through the JWKS route it publishes. The key is GENERATED HERE and
# never committed — prd gets a real one from its secret store, and each bench
# signing with its own means a token from one developer's machine is worthless on
# another's, which is the correct blast radius for a credential nobody rotates.
#
# Generated once and reused: regenerating on every start would invalidate every
# token from the previous run mid-session for no reason.
KEY_FILE="devops/dev-signing-key.pem"
if [[ ! -f "$KEY_FILE" ]]; then
  echo "start.sh: generating a dev signing key at $KEY_FILE (not committed)"
  mkdir -p "$(dirname "$KEY_FILE")"
  # PKCS#8, RSA 2048 — the Issuer rejects RSA under 2048 bits at construction,
  # and parses PKCS#8 specifically.
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$KEY_FILE" 2>/dev/null
  chmod 600 "$KEY_FILE"
fi

# THE NEWLINES MUST BE LITERAL BACKSLASH-N, and this is the whole reason this line
# is not a plain `$(cat ...)`.
#
# Config interpolation runs on the RAW FILE TEXT before the yaml is parsed, so a
# PEM with real newlines substituted into `privateKeyPem: "${JWT_SIGNING_KEY}"`
# produces a double-quoted scalar broken across lines — yaml then FOLDS those
# newlines into spaces and the key silently fails to parse. Feeding `\n` as two
# characters lets the double-quoted scalar decode them back into real newlines,
# which is the one form that survives both passes.
JWT_SIGNING_KEY="$(awk '{printf "%s\\n", $0}' "$KEY_FILE")"
export JWT_SIGNING_KEY

# Ties the token's `kid` to the key material. Change the file, change the kid, and
# a stale token stops validating instead of failing obscurely against a key that
# no longer matches.
JWT_SIGNING_KID="dev-$(openssl dgst -sha256 "$KEY_FILE" | awk '{print substr($NF,1,8)}')"
export JWT_SIGNING_KID

docker compose -f devops/docker-compose.yml up -d --wait
go build -tags 'postgres' -o ./bin/authcore ./bootstrap
APP_PROFILE=dev exec ./bin/authcore
