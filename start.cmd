@echo off
REM Bring up the local bench and run authcore in dev mode (always the dev profile).
REM Build + run the binary, never `go run`: the signal must reach the app itself.
REM The build carries the `postgres` engine tag and no transport tag, because the
REM yaml declares no `transport:` block.
cd /d "%~dp0"

docker compose -f devops/docker-compose.yml up -d --wait || exit /b 1
go build -tags "postgres" -o .\bin\authcore.exe .\bootstrap || exit /b 1
set "APP_PROFILE=dev"
.\bin\authcore.exe
