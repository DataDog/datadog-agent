# Mutation Testing — Logs Agent (Go)

Local bulk sweep harness for running [gremlins](https://github.com/go-gremlins/gremlins) mutation testing across all packages under `pkg/logs/` and `comp/logs/`.

Each surviving mutant is a concrete, line-level pointer to a test gap: a semantic code change that no existing test detected.

## Quick start

Requires `dda` (`brew install --cask dda`), Go, and tmux. The patched gremlins binary is built automatically on first run.

Single package:

```bash
python3 .gitlab/mutation-testing/run_mutation.py pkg/logs/status
```

Full sweep, detached (resumable):

```bash
.gitlab/mutation-testing/launch_all.sh
```

List all discovered packages and their classification without running:

```bash
python3 .gitlab/mutation-testing/run_mutation.py --list
```

Results land in `~/research/logs-agent-mutation-results/` by default — one `report.md` per package and a rolled-up `summary.md`.

## Why patched gremlins

Stock gremlins runs `go test -cover` to gather coverage before mutating. In datadog-agent this breaks for two reasons:

1. Many packages have tests that assert on `-ldflags`-set vars (e.g. `pkg/version`) — plain `go test` fails at baseline.
2. Most non-trivial packages need build tags computed by `dda inv test`.

The patch adds two flags:

- `--no-coverage` — skip the coverage pre-pass; mutate every site.
- `--test-cmd` — replace the hardcoded `go test` with an arbitrary command (here, a wrapper that `cd`s to the repo root and runs `dda inv -- -e test --targets=./<pkg>`).

The patch lives at `patches/0001-add-test-cmd-and-no-coverage-flags.patch` and is applied to upstream gremlins `v0.6.0` at build time by `run_mutation.py`.

## Interpreting results

- **Killed** — a test failed when the code was mutated. The test suite caught it.
- **Survived (LIVED / NOT_COVERED)** — all tests passed despite the mutation. No test asserts on that behavior.
- **Score** = `killed / (killed + survived)`. Aim for ≥ 75%.

Mutator codes: `CN` = conditional negation, `CB` = conditional boundary, `AR` = arithmetic, `ID`/`IN` = increment/decrement/invert-negative.

## Files

- `run_mutation.py` — resumable harness. Discovers targets, builds patched gremlins, runs one package at a time, writes per-package `report.md` and `summary.md`.
- `launch_all.sh` — tmux launcher for a full sweep. Safe to disconnect; re-run resumes where it left off.
- `muttest_render.py` — gremlins JSON → markdown renderer. Imported by `run_mutation.py`.
- `tests/test_render.py` — unit tests for the renderer.
- `patches/` — gremlins patch ported from [dd-source PR #428124](https://github.com/DataDog/dd-source/pull/428124).
- `skip_tags.txt` — documentation of build-tag-gated packages that the sweep skips.

## Renderer unit tests

```bash
PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 \
  python3 -m pytest .gitlab/mutation-testing/tests -v -p no:cacheprovider
```
