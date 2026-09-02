# QA round — id addresses and filter values

Status: APPROVED (2026-09-02)
Suite slug: `id-address-and-filter-values` — what this round PROVES.
Pin: omnicore **v0.70.0** (was v0.69.0 when the previous round was approved)
Scope: entity `Tenant`, surfaces REST + GraphQL

## What this round ADDS

The two contracts v0.70.0 introduced, neither of which the previous round could have a
case for — below that pin the same requests were a 500 on a relational backing:

- **A by-id ADDRESS that is not a uuid, split by VERB and not by surface.** A read
  answers 404 `UnknownIDAddressNotification`; a write answers 400
  `MalformedIDNotification`; identically on REST and GraphQL.
- **A filter VALUE outside the leaf's declared kind** → 400
  `InvalidFilterValueNotification`.
- **The report artifact** the runner now owes (`qa/qa-report.md`), which the previous
  round's runner does not write.

## What this round INHERITS, unchanged

[`specs/qa/tenant/plan.md`](../tenant/plan.md), `Status: APPROVED`, and every case family
in it: the six served verbs, the golden record, the seven 422s, both 409 flavors, the
archive round-trip, the whole read vocabulary and pagination envelope, the typed-400
guard family, routing, and the GraphQL parity sweep. **184 cases, all still green at
v0.70.0** unless this round's run says otherwise.

Inherited and NOT reopened: §2 data hygiene (throwaway `authcore_qa_db` via
`OMNICORE_CONFIG_PATH=qa/microservice.qa.yaml`), §3 auth (the service's own seeded
bootstrap admin, rotated), §5 runner contract (one `qa/run.sh`, `SUITES=(tenant)`,
fail-fast by default, SIGTERM + drain, per-lane namespacing).

There is still exactly ONE runner and ONE lane script. This round extends
`qa/tenant.sh`; it does not add a rival entry point.

---

## 1. Coverage this round adds

Derived from `status-mapping` and `changelog` at v0.70.0, then **probed against the
running service before being written as cases** — the previous round taught that a
mis-derived expectation costs a full run.

### K — the by-id address, by verb (pin ≥ v0.70.0)

Envelope, verified: context `Request`, `field: "id"`, `value` echoes the rejected
segment.

| case | request | expected |
|---|---|---|
| `I5` *(amended)* | `GET /tenants/not-a-uuid` | **404** `UnknownIDAddressNotification` |
| `K1` | `PATCH /tenants/not-a-uuid` (body) | **400** `MalformedIDNotification` |
| `K2` | `PATCH /tenants/not-a-uuid/archive` | **400** — the bodyless by-id command, the arm OpenAPI left undeclared before this pin |
| `K3` | `PATCH /tenants/not-a-uuid/unarchive` | **400** |
| `K4` | `value` echo on `K1` | `"not-a-uuid"`, and `context == "Request"` — an address problem is not labelled a payload problem |
| `K5` | GraphQL `tenant(id: "not-a-uuid")` | HTTP 200, `extensions.notificationKey == UnknownIDAddressNotification`, `semantic == NotFound` |
| `K6` | GraphQL `archiveTenant(id: "not-a-uuid")` | HTTP 200, `extensions.notificationKey == MalformedIDNotification`, `semantic == Schema` |

`K5`/`K6` are the ones that matter most: at v0.69.0 this surface answered an **untyped**
`{"semantic":"Internal"}` with no `notificationKey` at all, so a GraphQL consumer could
not tell a bad address from a server fault.

`I5`'s KEY changes and its STATUS does not. The previous round asserted 404
`RecordNotFoundNotification` by reasoning, at a pin whose docs promised nothing here.
v0.70.0 made 404 the documented contract but gave it its own key, because
`RecordNotFoundNotification` means a well-formed address matched no row (`I1`) — and the
two must stay distinguishable. The other six `RecordNotFoundNotification` assertions
(`F1`, `F6`, `F7`, `I1`, `J5b`, `J6`) all send well-formed UUIDs and are untouched.

### L — a filter value the leaf cannot take (pin ≥ v0.70.0)

| case | request | expected |
|---|---|---|
| `L1` | `?createdAt.gte=not-a-date` on a `*time.Time` leaf | **400** `InvalidFilterValueNotification` |
| `L2` | `?createdAt=not-a-date` (the `eq` operator, same leaf) | **400** `InvalidFilterValueNotification` |

⚠️ **Both are expected to stand RED against v0.70.0 — see §3.** They assert the pin's
documented promise, not the observed behavior. Per this skill's standing rule a case is
never weakened to pass: the finding is reported and the cases turn green when it is fixed
upstream.

Tenant declares no `int64`, `bool` or identity filter leaf, so the kinds that DO work at
this pin cannot be proven from this lane. They were verified by hand during the probe and
are recorded in §3 as evidence for the report, not smuggled in as cases against a route
this suite does not own.

### M — the report artifact

| case | what |
|---|---|
| `M1` | `qa/qa-report.md` exists after a run, and its footer matches the run's real verdict |
| `M2` | the deliberate-RED meta-case shows the suite RED **in the report**, with the failing case named and its real body — a report that stays green through a failing run hides every future failure |

`M2` is executed by hand as part of the final gate (break a case → run → inspect report →
restore), not as a self-referential case inside the suite.

## 2. Runner change — the report contract

`qa/run.sh` gains, per the skill's §6:

- a run id and `qa/.logs/<run-id>/` holding each lane's full stdout and server log;
- `qa/qa-report.md` **rewritten in full after every suite**, never only at the end, so a
  run killed halfway still leaves what it had proven;
- header (timestamp · profile + the tags actually BUILT · the omnicore pin · hygiene mode
  · suite count · this plan's path), a matrix row per DECLARED suite (a suite that never
  ran prints `—` and never vanishes), a failures section carrying expected vs received
  plus the real body, and a one-line footer echoed to stdout;
- a trap on `EXIT INT TERM` stamping `❌ RUN ABORTED — <reason>`, disarmed only once the
  final verdict is on disk.

`SKIP` keeps its own column and is never folded into the pass count.

`qa/qa-report.md` and `qa/.logs/` are RUN ARTIFACTS — reproducible by re-running the
command. They are OFFERED for `.gitignore` at hand-off and never added by this skill.

## 3. FINDING carried into this round — the temporal leaf still leaks an external 500

v0.70.0 promises: *"a filter value the leaf cannot take is refused instead of passed
through … now 400 `InvalidFilterValueNotification`, named by the wire key … so the probe
never reaches the driver."*

Probed against the running service at v0.70.0, authenticated, one leaf kind per row:

| request | leaf kind declared | answer |
|---|---|---|
| `users ?tenantID=lixo` | `*domain.ID` | 400 `InvalidFilterValueNotification` ✅ |
| `roles ?tenantID=lixo` | `*domain.ID` | 400 ✅ |
| `users ?mustChangePassword=abc` | `*bool` | 400 ✅ |
| `users ?id=lixo` | `*string` over an identity column (the reader-guarded path) | 400 ✅ |
| `tenants ?createdAt.gte=not-a-date` | `*time.Time` | **500 external** ❌ |
| `tenants ?createdAt=not-a-date` | `*time.Time` | **500 external** ❌ |

Server log on the two failures: `invalid input syntax for type timestamp with time zone`,
twice — the probe **reached the driver**, which is precisely what the pin says can no
longer happen. Every other kind is guarded correctly, so this is one missed kind, not a
feature that failed to land.

Second, smaller discrepancy in the same feature: the changelog promises the field is
*"named by the wire key"*, and two of the four working rows echo the **Go** field name
instead — `TenantID` and `ID`, against `mustChangePassword` which is correct.

Both belong to omnicore, not to this service: the guard, the leaf-kind classification and
the field naming are all framework code, and `omnicore-gen doctor` is clean. Written up
for forwarding in
[`finding-filter-value-temporal-leaf.md`](finding-filter-value-temporal-leaf.md).

## 4. Out of scope, unchanged from the inherited round

Load/performance, UI, the other eight entities. Integration events remain N/A (no
`transport:` block, no `integration_events` table). The `InvalidIDUUIDNotification` (422,
domain validation through `ID.IsValid`) is untouched by v0.70.0 and is not reachable from
any Tenant route, so it gets no case.

## 5. Gate decision (2026-09-02)

- **Q1 — `L1`/`L2` go in, and stand RED.** They assert the pin's documented promise, not
  the observed behavior; they turn green on their own when the temporal leaf is fixed
  upstream. A suite that ran all-green would be claiming v0.70.0 landed whole, which is
  not true.
