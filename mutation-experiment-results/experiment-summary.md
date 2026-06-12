# Spec-Derived Test Coverage: Mutation Testing Experiment

**Question:** Do the tests added on `cmetz/logs_agent_preprocessor_and_truncation_specs_and_tests` (the spec branch) kill any mutants that the pre-existing tests did not?

**Answer:** Yes. The spec-derived tests killed **11 previously-surviving mutants** in `pkg/logs/internal/decoder/preprocessor` (zero in `pkg/logs/internal/framer`). Of those 11, **9 came from non-proptest spec-derived tests** and **2 came uniquely from rapid-driven proptests**.

## Setup

| | |
|---|---|
| Harness | gremlins v0.6.0, patched per PR #51452 (adds `--test-cmd` and `--no-coverage`) |
| Driver | `.gitlab/mutation-testing/run_mutation.py` |
| Packages under test | `pkg/logs/internal/framer`, `pkg/logs/internal/decoder/preprocessor` |
| Rapid seed | 42 |
| Rapid checks | 1000 per property |
| Per-mutant timeout | 120s |
| Branches compared | merge base of (`origin/main`, spec branch) vs the spec branch tip |

The merge base is used (not `origin/main` directly) for an apples-to-apples comparison. Production code is byte-identical on both sides, so gremlins generates the same mutant set on both — any difference in killed/survived counts is purely test-coverage delta.

## Methodology

Three mutation runs, each with the same harness and seed:

1. **Merge base** (`cmetz/spec_mutation_experiment_mergebase`): no spec-derived tests present.
2. **Spec branch full** (`cmetz/spec_mutation_experiment`): all spec-derived tests present.
3. **Spec branch minus proptests** (`mutation-experiment-results/spec-branch-no-proptests/`): every `*_proptest_test.go` file moved out of the package directory.

This produces a three-way attribution: kills attributable to the non-proptest spec changes vs kills uniquely attributable to proptests.

## Results

| Package | Run | Killed | Survived | Score | Timeouts |
|---|---|---:|---:|---:|---:|
| framer | merge base | 104 | 32 | 76.5% | 13 |
| framer | spec branch | 104 | 32 | 76.5% | 13 |
| preprocessor | merge base | 321 | 86 | 78.9% | 1 |
| preprocessor | spec branch (no proptests) | 330 | 77 | 81.1% | 1 |
| preprocessor | spec branch (full) | 332 | 75 | 81.6% | 1 |

**Framer:** zero uplift. The added test file did not kill any new mutants.

**Preprocessor:** +11 kills total (+2.7 percentage points). 9 from non-proptest spec-derived tests, 2 from proptests on top.

## Attribution

### Killed by non-proptest spec-derived tests (9)

| File | Line | Mutator |
|---|---|---|
| `preprocessor.go` | 77 | CONDITIONALS_BOUNDARY |
| `preprocessor.go` | 77 | CONDITIONALS_NEGATION |
| `preprocessor.go` | 90 | CONDITIONALS_NEGATION |
| `preprocessor.go` | 111 | CONDITIONALS_NEGATION |
| `regex_aggregator.go` | 107 | INCREMENT_DECREMENT |
| `regex_aggregator.go` | 140 | CONDITIONALS_NEGATION |
| `regex_aggregator.go` | 181 | CONDITIONALS_BOUNDARY |
| `regex_aggregator.go` | 181 | CONDITIONALS_NEGATION |
| `tokenizer.go` | 151 | CONDITIONALS_BOUNDARY |

### Killed uniquely by proptests (2)

| File | Line | Mutator |
|---|---|---|
| `combining_aggregator.go` | 85 | CONDITIONALS_BOUNDARY |
| `token_graph.go` | 72 | CONDITIONALS_BOUNDARY |

Both proptest-only kills are `CONDITIONALS_BOUNDARY` — exactly the mutator type the mutation-testing rules doc calls out as the kind of gap proptests are best suited to close (random inputs hitting exact boundary values that hand-written tests miss).

## Interpretation

- **The spec-derived tests do add real mutation coverage** — 11 concrete `file:line:mutator` locations where pre-existing tests did not catch a semantic change and the new tests do.
- **Proptests contribute ~18% of the total uplift** (2 of 11). The bulk of the new coverage comes from the regular unit and contract tests added alongside the proptests, not from rapid-driven property checking.
- **Three plausible explanations for the modest proptest contribution:**
  1. The proptests are largely shadowed by the unit tests' line coverage of the same code paths — mutation testing only attributes a kill to "any test failed," so genuine new coverage that overlaps existing coverage is invisible here.
  2. The mutation operators gremlins models (CN, CB, AR, ID, IN — syntactic mutations) are not the bug class proptests are best at catching. Proptests catch invariant violations, distribution-level bugs, and multi-step state interactions that no syntactic mutation models.
  3. The non-proptest tests written alongside the proptests were already comprehensive enough that the proptests had little marginal surface left to cover.
- **Null result on framer is expected.** Only one new test file was added there and it was not a proptest.

## Caveats

- **gremlins's mutation operators are a narrow lens.** A 100% mutation score does not mean "no bugs possible" — it means "every line-level syntactic mutation the tool models is caught by at least one test." Bugs from state-machine invariants, distribution-level properties, or cross-call interactions are not modeled.
- **Proptests can kill mutations that unit tests also kill** — those kills are invisible in this attribution because mutation testing does not record which test killed a mutant, only that some test did. The reported proptest contribution (2 mutants) is the *uniquely attributable* count, not the total kills proptests achieved.
- **Timeouts count as kills.** A mutant that causes the test process to hang past 120s is recorded as killed (because `go test` exits nonzero). Framer has 13 such mutants on both runs; preprocessor has 1. They are unchanged between runs, so they do not affect the comparison.

## Reproduction

```bash
# 1. Branch off the merge base, cherry-pick the harness commit
BASE=$(git merge-base origin/main cmetz/logs_agent_preprocessor_and_truncation_specs_and_tests)
git checkout -b mut-mergebase "$BASE"
git cherry-pick <harness-commit>

# 2. Run merge-base baseline
python3 .gitlab/mutation-testing/run_mutation.py \
  --repo "$(pwd -P)" --scope pkg-logs \
  --results-dir mutation-experiment-results/merge-base \
  --rapid-seed 42 --rapid-checks 1000 \
  pkg/logs/internal/framer pkg/logs/internal/decoder/preprocessor

# 3. Switch to spec branch, run full
git checkout cmetz/spec_mutation_experiment
python3 .gitlab/mutation-testing/run_mutation.py \
  --repo "$(pwd -P)" --scope pkg-logs \
  --results-dir mutation-experiment-results/spec-branch \
  --rapid-seed 42 --rapid-checks 1000 \
  pkg/logs/internal/framer pkg/logs/internal/decoder/preprocessor

# 4. Run spec branch with proptests removed
mkdir -p /tmp/proptest-stash
mv pkg/logs/internal/decoder/preprocessor/*_proptest_test.go /tmp/proptest-stash/
python3 .gitlab/mutation-testing/run_mutation.py \
  --repo "$(pwd -P)" --scope pkg-logs \
  --results-dir mutation-experiment-results/spec-branch-no-proptests \
  --rapid-seed 42 --rapid-checks 1000 \
  pkg/logs/internal/decoder/preprocessor
mv /tmp/proptest-stash/* pkg/logs/internal/decoder/preprocessor/
```

The harness used here required two fixes vs the version in PR #51452, both committed on this branch:

1. **`GOWORK=off` in the test wrapper.** A `go.work` file higher in the directory tree was redirecting `go test` to the wrong module root, causing every test invocation to fail at build resolution. With the bug present every mutant reported as killed (100% score across the board) because `go test` returned nonzero on every invocation regardless of whether the mutation was caught.
2. **Rapid flag ordering.** The wrapper passed `-rapid.seed` and `-rapid.checks` before the package path. `go test` interprets unknown flags before the package path as `go test` flags and triggers a wider build resolution that fails on this repo. Moving the rapid flags after the package path (where they are passed through to the test binary) is the correct invocation. The wrapper also now skips the rapid flags entirely if no `*_test.go` in the target imports `pgregory.net/rapid`, so it works correctly at the merge base where no tests use rapid yet.

## Harness iteration log

The two fixes above did not land in a single change. The harness was iteratively debugged over several runs before producing trustworthy numbers. The pattern across all bug states was the same: gremlins reported 100% kill rate because `go test` was failing for a non-mutation reason on every invocation, and gremlins counts any nonzero exit as a killed mutant. The fix sequence:

1. **Initial run (broken).** Wrapper invoked `dda inv test` originally; switched to raw `go test -tags=test` to allow passing `-rapid.seed` directly (dda's argparser chokes on leading-dash flags). First runs of framer and preprocessor both reported 136/136 and 407/407 killed — implausibly clean.

2. **Diagnosis attempt #1: `unset GOWORK`.** Suspected the `go.work` file in the alternate checkout path (`/Users/caleb.metz/dd/datadog-agent`) was being picked up. Added `unset GOWORK` to the wrapper. Re-run still showed 100% kill rate; same bug.

3. **Diagnosis attempt #2: `export GOWORK=off`.** Switched from `unset` to explicit `off` since `go env GOWORK` was still returning a stale path even after `unset`. Re-run still showed 100% kill rate; same bug.

4. **Diagnosis via `mutant_runs.log`.** Inspected the per-invocation rc log and found every one of 407 invocations returned `rc=1 timeout=0`. Confirmed the wrapper was not correctly causing `go test` to succeed for the unmutated baseline. Ran the wrapper command manually and saw `package github.com/DataDog/datadog-agent: build constraints exclude all Go files in /Users/caleb.metz/dd/datadog-agent`.

5. **Diagnosis attempt #3: canonical path.** Passed `--repo "$(pwd -P)"` to resolve the symlinked path. The merge-base run now produced real results (76.5% / 78.9%) because rapid was not imported at the merge base and the wrapper did not pass the rapid flags. But the spec-branch run was still bogus (100% across the board).

6. **Root cause: rapid flag ordering.** Isolated by running `go test` with and without `-rapid.seed=42` manually. Adding the flag before the package path triggered the "build constraints exclude" error; adding it after the package path worked. `go test` interprets unknown flags before the package path as `go test` flags and tries to resolve them via a wider build context that fails on this repo. Fix: move rapid flags after the package path; also detect at runtime whether the target imports rapid and skip the flags entirely otherwise.

7. **Final clean run.** Three-way comparison produced the results documented above.

Lessons worth carrying forward:

- **A 100% mutation score on first run is a red flag, not a result.** Real-world Go packages rarely have perfectly comprehensive tests; the dominant explanation for 100% is "tests are not actually running."
- **`mutant_runs.log` is the first place to look when results look implausible.** Uniform rc values across hundreds of invocations indicates an environmental rather than test-quality issue.
- **`go test` flag ordering matters for non-stdlib test flags.** Custom test binary flags must come after the package path or via `-args`; before the package path, `go test` interprets them itself.
- **Symlinked working directories interact badly with go workspaces.** Even with `GOWORK=off` set, go can still find a workspace via the resolved-symlink path. Using `pwd -P` for tooling that depends on canonical module resolution avoids the class of bug entirely.
