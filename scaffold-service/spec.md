# scaffold-service — authcore

Status: APPROVED

The service-level model this shell was generated from. Every high-risk slot below was
answered interactively by the maintainer; the low-risk slots were decided and are shown
filled so they can be corrected here rather than reverse-engineered from the files.

## Identity

| Slot | Value | Source |
|---|---|---|
| Service name | `authcore` | maintainer (invocation) |
| Go module path | `github.com/ClaudioSchirmer/authcore` | maintainer (invocation) |
| Description | Identity and authentication for a multi-tenant platform. | README.md, first line |
| Working language (chat) | Portuguese | maintainer |
| Written language (all artifacts) | English | CLAUDE.md rule 3 |

## Infrastructure posture

| Slot | Value | Source |
|---|---|---|
| Relational dialect | `postgres` | maintainer |
| MongoDB | absent — no `mongo:` block | maintainer |
| Transport / broker | absent — no `transport:` block, no transport build tag | maintainer |
| CDC relay (Debezium) | none | follows from the two above |
| Read-side posture | relational-served (`.RelationalSource(repo.Loader)`) | maintainer |
| Build tags | `postgres` only | follows from the posture |

What the posture buys and what it costs, recorded so it is not rediscovered later:

- Reads are composed from the source of truth at read time — read-your-writes, no CDC lag.
- Free-text search (`?search=`) is answered with a typed 400
  (`RelationalCapabilityNotification`), as are filter/sort over 1:N child fields.
  Filter/sort over root columns, siblings and shared-base fields work normally.
- Integration events cannot be PUBLISHED — publishing rides the CDC relay, which does not
  exist here. CONSUMING another service's events would need only a broker plus the
  transport build tag.
- Multi-source read models (ComposedView, SharedBaseView, the Embed/Link family, Upstream)
  require Mongo and are unavailable.
- Fully reversible through `/omnicore:configure`, with no application code lost.

## Surfaces

| Surface | Decision | Notes |
|---|---|---|
| OpenAPI UI | yes — `uiPath /docs`, `rootRedirect: true` | `Wiring.OpenAPI` set with `LanguageSelector: true` |
| GraphQL | yes — endpoint `/graphql`, playground `/graphql/ui` (dev only) | INERT until the first entity opts in |
| gRPC | no | additive later, no rework |

## Local bench

| Slot | Value |
|---|---|
| `devops/docker-compose.yml` | generated — a single Postgres container, healthchecked |
| Compose project | `authcore-dev` |
| Container | `authcore-dev-postgres` |
| Mongo / broker / Debezium | not generated (posture) |
| Start wrappers | `start.sh` (host-native, darwin) + `start.cmd` + `start.ps1` |

## Low-risk slots (decided, not asked)

| Slot | Value |
|---|---|
| omnicore version | latest published release, resolved at generation time (recorded below) |
| Go toolchain | 1.26.5 (host) |
| HTTP port | `8080` (standard; no collision found in the Phase 0 port scan) |
| Postgres host port | `5432` (standard; free) |
| Postgres credentials | `omnicore:omnicore` |
| Relational database | `authcore_db` |
| Relational DSN default | `postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable` |
| Migrations dir | `./migrations/postgres` |
| `migrations.autoRun` | dev default (`true` in dev, `check` elsewhere) |
| Audit | no `audit:` block — the framework default `[slog, database]` applies |
| Auth (dev) | `mode: disabled` (accepted only under `APP_PROFILE=dev`) |
| Auth (prd) | `mode: jwt` with `${JWT_ISSUER}` / `${JWT_AUDIENCE}` / `${JWKS_URL}`, and `publicRoutes: ["GET /livez", "GET /readyz"]` |
| Shutdown | framework defaults |

Every endpoint is written as `${VAR:default}` in the dev profile so it can be repointed
without editing the YAML; the prd profile uses bare `${VAR}` with no localhost defaults.

## Resolved at generation time

| Slot | Value |
|---|---|
| omnicore version | `v0.53.0` |

## Out of scope for this run

No entities. The shell boots empty by design; the first aggregate is
`/omnicore:scaffold-entity`'s job.

## Verification (final gate, run at generation time)

| Check | Result |
|---|---|
| Bench healthy | `authcore-dev-postgres` Up (healthy), `5432` published |
| `gofmt -l .` | clean |
| `go vet -tags postgres ./...` | clean |
| `go build -tags postgres ./bootstrap` | OK (`go.mod` + `go.sum` both shipped) |
| Boot (`APP_PROFILE=dev`, via `./start.sh`) | `GET /livez` 200 · `GET /readyz` 200 |
| OpenAPI surface | `GET /docs` 200 · `GET /openapi.json` 200 · `GET /` 302 → `/docs` |
| GraphQL surface | block present; `/graphql/ui` 404 — the surface stays unmounted until a feature implements `bootstrap.GraphQLFeature` (expected on an empty shell) |
| Empty-shell warning | present, as designed: `wiring declared no Features and no BeforeServe — dev-only empty-shell boot` |
| Posture confirmed at boot | `mongo disabled — no mongo.uri configured (relational views only; no Mongo projections, no CDC)` |
| Framework migrations applied | 8 control-plane tables in `authcore_db` (`outbox`, `audit_events`, `omnicore_framework_migrations`, …). `omnicore_migrations` absent — correct: the service sequence is still empty |
| Graceful shutdown | SIGTERM → drain narration complete, exit 0 |
| prd profile (static only — never boot-tested) | decodes through `bootstrap.LoadConfigFrom`: `auth.mode=jwt`, `publicRoutes=[GET /livez, GET /readyz]`, `autoRun=check`, DSN a bare `${DATABASE_URL}` with no localhost fallback |

Deviations from the values named while the spec was being agreed:

- Postgres image is `postgres:17-alpine`. The plugin's bench template references
  `postgres:16-alpine`; the tag is not framework-facing, and 17 is what was offered.
- The `shutdown:` block is written out explicitly. Its values are the framework
  defaults, so behavior is unchanged — it is there to document the drain budget.
