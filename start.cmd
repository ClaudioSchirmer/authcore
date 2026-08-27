@echo off
REM Bring up the local bench and run authcore in dev mode (always the dev profile).
REM Build + run the binary, never `go run`: the signal must reach the app itself.
REM The build carries the `postgres` engine tag and no transport tag, because the
REM yaml declares no `transport:` block.
setlocal enabledelayedexpansion
cd /d "%~dp0"

REM ── the dev signing key ─────────────────────────────────────────────────────
REM Mirror of start.sh and start.ps1, and it must stay in step with them: this
REM service is its own IdP, so a bench without JWT_SIGNING_KEY fails the boot at
REM NewIssuer rather than starting insecurely. Generated once, never committed.
set "KEY_FILE=devops\dev-signing-key.pem"
if not exist "%KEY_FILE%" (
  echo start.cmd: generating a dev signing key at %KEY_FILE% ^(not committed^)
  if not exist "devops" mkdir "devops"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "%KEY_FILE%" || exit /b 1
)

REM THE NEWLINES MUST BE LITERAL BACKSLASH-N. Config interpolation runs on the raw
REM file text before the yaml is parsed, so a PEM with real newlines substituted
REM into privateKeyPem: "${JWT_SIGNING_KEY}" yields a double-quoted scalar broken
REM across lines — yaml folds those newlines into spaces and the key silently
REM fails to parse. Two-character \n survives both passes.
set "JWT_SIGNING_KEY="
for /f "usebackq delims=" %%L in ("%KEY_FILE%") do set "JWT_SIGNING_KEY=!JWT_SIGNING_KEY!%%L\n"

REM Ties the token's kid to the key material, so a stale token stops validating
REM instead of failing obscurely against a key that no longer matches.
for /f "usebackq skip=1 tokens=1 delims= " %%H in (`certutil -hashfile "%KEY_FILE%" SHA256`) do (
  if not defined JWT_SIGNING_KID set "JWT_SIGNING_KID=dev-%%H"
)
set "JWT_SIGNING_KID=!JWT_SIGNING_KID:~0,12!"

docker compose -f devops/docker-compose.yml up -d --wait || exit /b 1
go build -tags "postgres" -o .\bin\authcore.exe .\bootstrap || exit /b 1
set "APP_PROFILE=dev"
.\bin\authcore.exe
