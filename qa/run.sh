#!/usr/bin/env bash
#
# THE runner for authcore's contract QA suite. There is exactly one, and this is it:
# a dev learns a single command and CI wires a single line.
#
#   ./qa/run.sh              every lane, fail-fast (stops at the first RED)
#   ./qa/run.sh --all        every lane, exhaustive sweep (runs them all, reports at the end)
#   ./qa/run.sh tenant       one lane
#   ./qa/run.sh --all tenant claim
#
# Each lane is SELF-CONTAINED: it provisions its own throwaway database, builds and
# boots its own server on its own port, asserts, then drains that server with SIGTERM
# and waits for it before returning.
#
# EVERY RUN LEAVES A DOCUMENT: qa/qa-report.md, rewritten in full after each suite (not
# only at the end), plus the full logs under qa/.logs/<run-id>/. A run whose only trace
# is terminal scrollback is a run nobody can read afterwards.
#
# ADDING A LANE: create qa/<entity>.sh AND add <entity> to SUITES below, in the same
# change. `ls qa/*.sh` minus run.sh must equal SUITES exactly.
#
# plans: specs/qa/tenant/plan.md · specs/qa/id-address-and-filter-values/plan.md
set -uo pipefail

# The project root, resolved from THIS script's location — never from the caller's
# working directory, so devops/, migrations/ and the build resolve however it is invoked.
cd "$(dirname "$0")/.." || exit 1

SUITES=(tenant)
PLAN="specs/qa/id-address-and-filter-values/plan.md"

REPORT="qa/qa-report.md"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
LOG_DIR="qa/.logs/${RUN_ID}"
mkdir -p "$LOG_DIR"

STARTED_AT="$(date '+%Y-%m-%d %H:%M:%S %Z')"
START_EPOCH=$SECONDS
FINALIZED=0

FAIL_FAST=1
REQUESTED=()
for arg in "$@"; do
  case "$arg" in
    --all) FAIL_FAST=0 ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) printf 'run.sh: unknown flag %s\n' "$arg" >&2; exit 2 ;;
    *)  REQUESTED+=("$arg") ;;
  esac
done

if [[ ${#REQUESTED[@]} -gt 0 ]]; then
  for want in "${REQUESTED[@]}"; do
    found=0
    for s in "${SUITES[@]}"; do [[ "$s" == "$want" ]] && found=1; done
    [[ $found -eq 1 ]] || { printf 'run.sh: no such suite %s (known: %s)\n' "$want" "${SUITES[*]}" >&2; exit 2; }
  done
  LANES=("${REQUESTED[@]}")
else
  LANES=("${SUITES[@]}")
fi

for lane in "${LANES[@]}"; do
  [[ -x "qa/${lane}.sh" ]] || { printf 'run.sh: qa/%s.sh is missing or not executable\n' "$lane" >&2; exit 2; }
done

# ── the environment the report header must state, resolved once ──────────────
# The tags are not a guess: the dialect comes from the yaml the lanes actually boot with,
# and the absence of a transport: block is itself the declaration of the no-op adapter.
QA_CONFIG="qa/microservice.qa.yaml"
DIALECT="$(sed -n 's/^[[:space:]]*dialect:[[:space:]]*//p' "$QA_CONFIG" | head -1)"
if grep -qE '^transport:' "$QA_CONFIG"; then TRANSPORT="$(sed -n 's/^[[:space:]]*kind:[[:space:]]*//p' "$QA_CONFIG" | head -1)"; else TRANSPORT=""; fi
BUILD_TAGS="${DIALECT}${TRANSPORT:+ $TRANSPORT}"
PIN="$(go list -m -f '{{.Version}}' github.com/ClaudioSchirmer/omnicore 2>/dev/null || echo unknown)"

# ── per-lane state, parallel arrays keyed by index into SUITES ───────────────
declare -a R_PASS R_FAIL R_SKIP R_VERDICT R_TIME
for i in "${!SUITES[@]}"; do
  R_PASS[$i]="—"; R_FAIL[$i]="—"; R_SKIP[$i]="—"; R_VERDICT[$i]="—"; R_TIME[$i]="—"
done
index_of() { local n=$1 i; for i in "${!SUITES[@]}"; do [[ "${SUITES[$i]}" == "$n" ]] && { echo "$i"; return; }; done; echo -1; }

# ── the report ───────────────────────────────────────────────────────────────
# Rendered in FULL after every suite, so a run killed halfway still leaves what it had
# proven. $1 is the footer line; everything above it is rebuilt from the arrays.
render_report() {
  local footer="$1" elapsed=$((SECONDS - START_EPOCH)) i
  {
    printf '# QA contract report — authcore\n\n'
    printf '%s\n\n' "$footer"
    printf '| | |\n|---|---|\n'
    printf '| Run | `%s` |\n' "$RUN_ID"
    printf '| Started | %s |\n' "$STARTED_AT"
    printf '| Elapsed | %ss |\n' "$elapsed"
    printf '| Profile | `dev` via `OMNICORE_CONFIG_PATH=%s` |\n' "$QA_CONFIG"
    printf '| Built with | `-tags '"'"'%s'"'"'` |\n' "$BUILD_TAGS"
    printf '| omnicore pin | `%s` |\n' "$PIN"
    printf '| Data hygiene | throwaway `authcore_qa_db`, dropped and recreated per run |\n'
    printf '| Suites declared | %d |\n' "${#SUITES[@]}"
    printf '| Plan | [`%s`](../%s) |\n' "$PLAN" "$PLAN"
    printf '| Logs | `%s/` |\n\n' "$LOG_DIR"

    printf '## Matrix\n\n'
    printf '| Suite | Pass | Fail | Skip | Verdict | Time |\n'
    printf '|---|---:|---:|---:|---|---:|\n'
    for i in "${!SUITES[@]}"; do
      printf '| %s | %s | %s | %s | %s | %s |\n' \
        "${SUITES[$i]}" "${R_PASS[$i]}" "${R_FAIL[$i]}" "${R_SKIP[$i]}" "${R_VERDICT[$i]}" "${R_TIME[$i]}"
    done
    printf '\n'
    printf '_A suite that never ran prints `—` rather than vanishing: a missing row reads exactly like a suite that passed._\n'

    # Failures, only when there are any. This section is the whole reason the report
    # exists — it has to be enough to diagnose from without opening the logs.
    local any=0
    for i in "${!SUITES[@]}"; do
      [[ "${R_FAIL[$i]}" =~ ^[0-9]+$ && "${R_FAIL[$i]}" -gt 0 ]] && any=1
    done
    if [[ $any -eq 1 ]]; then
      printf '\n## Failures\n'
      for i in "${!SUITES[@]}"; do
        local f="${LOG_DIR}/${SUITES[$i]}.failures"
        [[ -s "$f" ]] || continue
        printf '\n### %s\n\n' "${SUITES[$i]}"
        cat "$f"
        printf '\nFull log: `%s/%s.log` · server log: `%s/%s-server.log`\n' \
          "$LOG_DIR" "${SUITES[$i]}" "$LOG_DIR" "${SUITES[$i]}"
      done
    fi
  } > "$REPORT"
}

# Traps, so a killed run cannot leave yesterday's verdict standing as today's. Disarmed
# only once the real footer is on disk.
#
# SIGNAL AND EXIT ARE SEPARATE HANDLERS ON PURPOSE, and the signal one EXITS. Bash defers
# a trap until the running foreground command returns, so a handler that only renders and
# falls through lets the loop carry on and overwrite the ABORTED stamp with a normal
# verdict — the run then reports as if nothing had happened. Exiting from the handler is
# what makes the stamp stick.
#
# The lane itself is not killed from here: under a real Ctrl-C the signal reaches the
# whole foreground process group, so the lane gets it too and its OWN exit trap drains the
# server with SIGTERM. Reaching in to kill it would race that drain.
ABORTED=0
stamp_aborted() {
  ABORTED=1
  render_report "❌ **RUN ABORTED** — ${1} · logs: \`${LOG_DIR}/\`"
  printf '\n❌ RUN ABORTED — %s — logs: %s/\n' "$1" "$LOG_DIR"
}
on_signal() {
  [[ $FINALIZED -eq 1 || $ABORTED -eq 1 ]] && exit 130
  stamp_aborted "interrupted"
  exit 130
}
on_exit() {
  local rc=$?
  [[ $FINALIZED -eq 1 || $ABORTED -eq 1 ]] && return
  stamp_aborted "exit ${rc}"
}
trap on_signal INT TERM
trap on_exit EXIT

render_report "⏳ **RUNNING** — started ${STARTED_AT}"

printf '\n══ authcore contract QA ══  run %s  lanes: %s  (%s)\n' \
  "$RUN_ID" "${LANES[*]}" "$([[ $FAIL_FAST -eq 1 ]] && echo fail-fast || echo exhaustive)"

OVERALL=0
TOTAL_CASES=0
for lane in "${LANES[@]}"; do
  i=$(index_of "$lane")
  printf '\n─── %s ───\n' "$lane"
  lane_start=$SECONDS

  # The lane writes its counts and failure blocks here; the runner never parses prose.
  QA_LOG_DIR="$LOG_DIR" QA_RUN_ID="$RUN_ID" "qa/${lane}.sh" 2>&1 | tee "${LOG_DIR}/${lane}.log"
  rc=${PIPESTATUS[0]}
  lane_time=$((SECONDS - lane_start))

  if [[ -r "${LOG_DIR}/${lane}.counts" ]]; then
    read -r p f s < "${LOG_DIR}/${lane}.counts"
  else
    p=0; f=1; s=0   # a lane that died before writing its counts did not pass
  fi
  R_PASS[$i]=$p; R_FAIL[$i]=$f; R_SKIP[$i]=$s; R_TIME[$i]="${lane_time}s"
  TOTAL_CASES=$((TOTAL_CASES + p + f + s))
  if [[ $rc -eq 0 && "$f" -eq 0 ]]; then R_VERDICT[$i]="✅ GREEN"; else R_VERDICT[$i]="❌ RED"; OVERALL=1; fi

  render_report "⏳ **RUNNING** — ${lane} done, started ${STARTED_AT}"

  if [[ $OVERALL -eq 1 && $FAIL_FAST -eq 1 ]]; then
    printf '\nrun.sh: %s failed — stopping (pass --all to sweep every lane)\n' "$lane"
    break
  fi
done

ELAPSED=$((SECONDS - START_EPOCH))
RAN=0; RED=0
for i in "${!SUITES[@]}"; do
  [[ "${R_VERDICT[$i]}" == "—" ]] && continue
  RAN=$((RAN+1)); [[ "${R_VERDICT[$i]}" == "❌ RED" ]] && RED=$((RED+1))
done

if [[ $OVERALL -eq 0 ]]; then
  FOOTER="✅ **ALL GREEN** — ${RAN}/${#SUITES[@]} suites · ${TOTAL_CASES} cases · ${ELAPSED}s"
else
  FOOTER="❌ **RED** — ${RED} of ${RAN} suites — logs: \`${LOG_DIR}/\`"
fi
render_report "$FOOTER"
FINALIZED=1

printf '\n%s\n' "$(printf '%s' "$FOOTER" | sed 's/\*\*//g')"
printf 'report: %s\n' "$REPORT"
exit $OVERALL
