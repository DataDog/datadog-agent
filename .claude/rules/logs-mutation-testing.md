---
globs: "pkg/logs/**/*.go, comp/logs/**/*.go"
---

# Mutation Testing — Logs Agent

Mutation testing is available for all packages under `pkg/logs/` and `comp/logs/`. Use it to find concrete test gaps: each surviving mutant is a `file:line` where no existing test catches a semantic change to the code.

## When to use it

- After writing or strengthening tests: run it on the affected package to verify the new assertions actually kill the target mutants.
- When fixing a bug: confirm the tests would have caught the regression before the fix.
- When asked to improve test coverage in a package: run it first to get a concrete worklist instead of guessing what's missing.

## Running it

Requires `dda` (`brew install --cask dda`), Go, and tmux. The patched gremlins binary is built automatically on first run.

Single package:

```bash
python3 .gitlab/mutation-testing/run_mutation.py pkg/logs/status
```

Full sweep (runs detached in tmux, resumable):

```bash
.gitlab/mutation-testing/launch_all.sh
```

Results land in `~/research/logs-agent-mutation-results/` by default — one `report.md` per package listing every surviving mutant with `file:line:mutator`.

List all discovered packages and their classification without running:

```bash
python3 .gitlab/mutation-testing/run_mutation.py --list
```

## Interpreting results

- **Killed** — a test failed when the code was mutated. The test suite caught it.
- **Survived (LIVED / NOT_COVERED)** — all tests passed despite the mutation. No test asserts on that behavior.
- **Score** = `killed / (killed + survived)`. Aim for ≥ 75%.

The dominant surviving mutator types across this codebase are:

- **CN (conditional negation)** — a conditional was flipped (e.g. `>` → `<=`) and no test failed. Both branches of the conditional were never independently asserted.
- **CB (conditional boundary)** — a boundary was shifted by one (e.g. `<` → `<=`) and no test failed. The exact boundary value was never pinned by a test.
- **AR (arithmetic)** — an arithmetic operation was changed and no test caught it.
- **ID / IN (increment/decrement / invert-negative)** — a `++`/`--` or sign flip went undetected.

A CN or CB survivor at `launcher.go:396` means: write a test that passes a value exactly at that boundary and asserts the outcome on both sides.

## Packages skipped by the sweep

Packages requiring non-default build tags (docker, kubelet, systemd, windows, serverless) are skipped — `go list -tags=''` fails for them. They need a separate run under the matching tag set, which is out of scope for the default sweep.
