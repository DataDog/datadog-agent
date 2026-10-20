# Proof tiers (cheap first; ask before tiers 2 and 3)

## Tier 1 (default, free, minutes)
- `dda inv test --targets=./comp/healthplatform/issues/<pkg>` (unit: `issue_test.go`, `check_test.go`, `detect_test.go`, `module_test.go`) and `dda inv test --only-modified-packages` for the root package.
- Registration integration test `comp/healthplatform/<pkg>_integration_test.go` (build tag `test`). For Path A it must use string literals for IssueID/IssueName/IssueType and must NOT import the issue package: an import runs its `init()`, registers the module, and the test would pass even with the `bundle.go` blank import deleted (mutation-tested). It runs `Bundle()` with a fakeintake, drives the real runner (Path A) or `store.ReportIssue` (Path B), and asserts on the received payload the worker-required fields: issue name, issue type, category, severity, title, description, and remediation steps. A second test enables the gate through config and environment only and expects the issue on the first scheduler tick. Title or steps empty means the registry fell back to the minimal proto (missing `bundle.go` import or template error). Prerequisite once per fresh worktree: `dda inv schema.codegen`. `dda inv test` sets the tags itself; do not use raw `go test` (the one exception is the scratch fixture generator).
- Resolve proof: show the issue clears when the condition clears, at unit level (`Run` stops returning the instance) and through the scheduler or `scheduler/mock` with an injectable interval and hold window. A checked-in skipped test is not proof. For hysteresis above 1 also prove a found but unconfirmed instance returns an error (so a restart does not resolve a real issue).
- Path B: no registration test applies (no `bundle.go` import). Use owner adapter tests (report on detect, resolve on clear, first evaluation resolves a persisted issue, resolve ids persisted under another id, resolve all on disable, nil store), the wiring test below (a requirement), the integration test with the real store (NEW then RESOLVED on the wire), and the Path B mutant table below. Run the fx graph test of each binary that wires the owner. Details: `references/path_b.md`.
- Run `bash scripts/verify_all.sh ...` for the whole tier-1 set; it prints one JSON summary and per-gate logs.
- `check_signal.py` check 17 confirms such a test exists.

## Wiring test recipe (Path B requirement)
Without it, the line that attaches the observer to the reporter and the disabled-owner hook line can each be deleted with the whole owner package green (this survived a mutation run). Drive the REAL owner entry point with a fake store:
1. Config overrides: the threshold knob set to 1 (or the smallest value that trips the condition), a short scan period, a temp run path.
2. A temp dir with two files and one wildcard source matching both; set the owner's optional store field to the fake store and the hostname to a fake before the owner starts; start the owner pipeline.
3. `require.Eventually` (5 s, 20 ms): the fake store has exactly one active id with the issue's base prefix (reported).
4. Remove one file; `require.Eventually` the active set is empty (resolved).
5. A sibling case with a nil store: it starts, does not panic, and reports nothing (the optional dependency path).
6. A disabled-owner case: construct the owner disabled with a fake store that holds one active issue and a lifecycle recorder; run the appended start hook; assert the store is empty.
The owner's tag set may exclude some builds: put the test under the same tag as its caller.

Wiring dependency link (Path B): the recipe must also include a test that fails when the dependency link is removed, that is, when the owner's fx `Requires` field (HealthPlatform) is no longer handed to the store field of the reporter. A test that only drives the observer with an already built store passes with that link deleted. `check_signal.py` check 34 warns when the wiring tests exercise the observer but never the `Requires` link.

Mutation honesty: a mutant counts as killed only when a test fails because of it. A compile error, a vet failure or a timeout is not a kill. The PR body lists per mutant the failing test; never write that all mutants were killed without that list.

## Mutant tables (every mutant must be killed, or listed as surviving with a reason in the PR body)
Run them with `verify_all.sh --mutation-cmd '<cmd>'`: each command applies one mutation, runs the named test, and restores the files (the script checks the tree is byte-identical afterwards). The observed exit code is recorded, never assumed. Adapt the names to your condition; the rows are the minimum.

| Path B mutant | Killed by |
|---|---|
| predicate flip (`>=` to `>`, or the boundary your detection mirrors) | owner or observer boundary test |
| transition dedupe removed (notify every evaluation) | transition and multi-source flip test |
| reconcile removed (other active ids never resolved) | adapter reconcile test |
| host fallback removed (helper returns an empty string) | helper unit test asserting on the helper itself |
| observer-attach wiring line removed | wiring test, reported case |
| disabled-owner hook call removed | wiring test, disabled case |
| `ReportIssue` removed or made a no-op | adapter tests |
| cancellation check before the callback removed | cancellation test |

| Path A mutant | Killed by |
|---|---|
| `bundle.go` blank import removed | registration test (fails on empty steps) |
| check body always returns healthy (`nil, nil`) | detect test against a real object |
| presence gate inverted or dropped | gate test or detect test |
| hysteresis or confirmation dropped | hysteresis test |
| transient error swallowed (returns `nil, nil` on a timeout) | error-path test |
| unconfirmed instance returns nothing instead of an error | scheduler-level test via `scheduler/mock` |

## Tier 2 (opt-in): AWS e2e
Needs the `account-admin-8h` role on the agent-sandbox account, `dda inv e2e.setup --team=<team>` done once (never run the interactive form), and real EC2 cost. Generate the test by reading `test/new-e2e/tests/agent-health/.claude/skills/write-agent-health-e2e/SKILL.md` in the worktree and following it for the module (read it by path; it is directory-scoped). Run: `dda inv new-e2e-tests.run --targets=./tests/agent-health/... --run=^Test<Module>Suite$`. Use `fixtures/30_seconds_healthplatform_forwarder.yaml` style config so egress is fast. Never provision without an explicit yes naming the cost.

## Tier 3 (opt-in): staging agent plus API
Run an agent against datad0g with `health_platform.enabled: true` and a short `health_platform.forwarder.interval`, reproduce the failure (I3), then query the Recommendations API with `insight_types` set and `scope.agent_health.issue_name` equal to the exact IssueName (auth via `dd-auth`; JWT header per the query how-to). Expect the issue to appear; do not assert `OPEN` from the worker (it emits `STATE_UNKNOWN` for active issues).

Always report which tier was reached and what was not run.

## Run hygiene (from the first backtests)
- Use the renderer's printed `run_hint` when `dda inv test --help` lists `--test-args` (current main); on older SHAs use `--test-run-name='^Name$' -v`. Confirm `--- PASS: <name>` in the log (run gazelle first or the file is not in the target).
- Foreground for a single debugging run of `dda inv test` or the linter; `verify_all.sh` runs the gates in parallel after gazelle. Scope to the new package, any owner directories, plus `./comp/healthplatform`.
- Mutation check is part of tier 1: Path A, the `bundle.go` import removal plus the check-body mutants; Path B, the full mutant table above (two deletion mutants are not enough). Confirm each mutated run FAILS, restore, and record the exit code you observe. Survivors go in the PR body.
