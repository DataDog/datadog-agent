---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# Antithesis Runs

## Run 1 — backpressure-no-rotation-loss demonstration

- **runId:** `226498823ce20b84c7b05575d6f2e30b-54-9`
- **launched:** 2026-05-29
- **webhook:** basic_test
- **duration:** 30 min
- **source:** datadog-agent
- **test-name:** datadog-agent/antithesis-demo-rotation-loss (submitted test-name; historical)
- **branch:** `blt/antithesis-file-tailer` (gt stack, not pushed; stack re-laid by subsystem 2026-05-29)
- **config image:** snouty-config:20260529-003731-d8f8
- **service image:** logs-antithesis-rotation-demo@sha256:3882afae86dd...
- **property under demonstration:** `backpressure-no-rotation-loss` (Always assertion;
  expected to FAIL, demonstrating silent log loss on file rotation under backpressure)
- **local pre-flight:** the SUT binary emitted `setup_complete`, a `Reachable` (hit),
  and the `Always` assertion with `condition:false` (written=50, delivered=3,
  dropped=47); `snouty validate` succeeded ("Setup-complete event detected").
- **triage (2026-05-29, run completed):**
  - **`backpressure-no-rotation-loss` (Always) → FAILING**, `counterexample_count: 41043`.
    Counterexample log (vtime 15.3788, container `logs-rotation-demo`): the `Always`
    assertion fired `condition:false` with `details {written:50, delivered:3, dropped:47}`.
  - **`rotation-under-backpressure experiment completed` (Reachable) → PASSING**
    (41043 examples) — the path was genuinely exercised; the failure is not vacuous.
  - Health meta-properties PASSING: `Build Succeeded`, `Assertions are present in
    customer code`, `No Antithesis session errors`, `No unexpected crashes`.
  - **Caveat:** `Software was instrumented` / `Symbols were uploaded` → FAILING because
    the SUT binary was copied prebuilt (no `antithesis-go-instrumentor` pass) — so the
    run had no coverage-guided steering or symbolization. SDK assertions worked fully
    regardless (the bug is deterministic). To deepen future runs, build via the
    instrumentor and ship `/symbols`.
  - `node - kill/pause/throttle` meta-properties FAILING (those faults not exercised
    in this single-container run) — consistent with research note A7.
  - triage_report URL captured in the run `show` output (auth-token link).

## Run 2 — two bugs (rotation-loss + seek-error)

- **runId:** `3fb8dbbeeacb7ac0f71aa870716e2f60-54-9`
- **launched:** 2026-05-29
- **duration:** 30 min; source datadog-agent; test datadog-agent/antithesis-demo-logs-bugs
- **branch:** `blt/antithesis-file-tailer` (gt stack, not pushed; stack re-laid by subsystem 2026-05-29)
- **properties (both Always, expected FAIL):**
  - `backpressure-no-rotation-loss` (written=50/delivered=3)
  - `offset-no-regression-on-seek-error` (resume line 10 ignored; whole file re-read)
- **local pre-flight:** both demo tests FAIL-as-expected; `snouty validate` OK.
- **triage (completed):** both `Always` properties **FAILING** —
  `backpressure-no-rotation-loss` 39,143 counterexamples; `offset-no-regression-on-seek-error`
  39,673 counterexamples. The `seek-error resume experiment completed` Reachable
  companion PASSED (39,673 examples) — path genuinely exercised.


