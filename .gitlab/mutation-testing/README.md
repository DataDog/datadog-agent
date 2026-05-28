# Mutation Testing — `pkg/` (Go)

Advisory mutation testing for Go packages changed under `pkg/`, run as a
non-blocking GitLab CI job. Posts a sticky PR comment with the mutation
report and uploads `mutation-report.md` as a GitLab artifact.

- Tool: [gremlins](https://github.com/go-gremlins/gremlins) with a patch
  porting [`--test-cmd` and `--no-coverage`](./patches/) from dd-source PR
  [#428124](https://github.com/DataDog/dd-source/pull/428124).
- Status: exploratory POC. Allowed to fail. No team or stakeholder owning it yet.

## Why patched gremlins

Stock gremlins runs `go test -cover` to gather coverage and uses the result to
decide which mutation sites are worth running tests against. In datadog-agent
this breaks for two reasons:

1. Many `pkg/*` modules have tests that assert on `-ldflags`-set vars (e.g.
   `pkg/version`) — plain `go test` fails baseline.
2. Most non-trivial packages need build tags computed by `dda inv test`.

The patched gremlins adds:

- `--no-coverage` — skip the coverage step; mutate every site.
- `--test-cmd "dda inv -- -e test --targets=./<pkg>"` — replace the hardcoded
  `go test` invocation.

Both flags are required together. The patch lives at
`patches/0001-add-test-cmd-and-no-coverage-flags.patch` and is applied to
upstream gremlins `v0.6.0` at build time by `muttest.sh`.

## Behavior

- Runs only on `*.go` files changed under `pkg/` vs `origin/main`.
- Groups changed files into Go packages (parent directories).
- Skips any package whose `go list -tags=''` fails — these need non-default
  build tags (system-probe, eBPF, kubelet, docker, …) and won't baseline-pass
  in stock CI.
- Caps at `MAX_MUTATION_UNITS=3` packages. Beyond the cap the job exits clean.
- Invokes patched gremlins per package with `dda inv test` as the test command.
- Never blocks merge (`allow_failure: true`).
- Produces `mutation-report.md` as a GitLab artifact.
- Posts a sticky PR comment titled "Mutation Testing Results" via
  `dda inv github.pr-commenter`, using the same dd-octo-sts token flow
  as the `golang_deps_commenter` job. The comment is deleted (via
  `--force-delete`) when a run produces no report.

## Files

- `muttest.sh` — pipeline entry point. Diffs packages, builds patched
  gremlins, runs per package, renders.
- `muttest_render.py` — gremlins-JSON → markdown report renderer.
- `mutation-testing.yml` — GitLab CI job definitions (mutation run + commenter).
- `tests/test_render.py` — unit tests for the renderer.
- `patches/` — gremlins patch ported from dd-source PR #428124.
- `skip_tags.txt` — documentation of expected build-tag-gated skips.

## Run locally

Requires `dda` (`brew install --cask dda`) and Go ≥ 1.25.

```bash
.gitlab/mutation-testing/muttest.sh --base-ref origin/main
```

For trivial single-module packages that don't need `dda inv` (e.g.
`pkg/util/backoff`):

```bash
.gitlab/mutation-testing/muttest.sh --base-ref origin/main --no-dda
```

To force-run on every package under `pkg/` (slow):

```bash
.gitlab/mutation-testing/muttest.sh --all
```

## Renderer unit tests

```bash
PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 \
  python3 -m pytest .gitlab/mutation-testing/tests -v -p no:cacheprovider
```

## Interpreting the report

Each surviving mutant is a concrete code change that no test detected.
For example, a `CONDITIONALS_BOUNDARY` mutant flipping `<` to `<=`
means no test exercises the boundary value. Use the file/line in the
report as the starting point for a new test.

A score below 75% on a touched package indicates significant test gaps,
following the threshold used by the parallel Python mutation-testing
sweep in integrations-core.
