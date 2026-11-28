---
name: write-test
description: Write the Go test a Datadog Agent change needs, at the right level — a unit test, an integration test wiring the real components with fx and a local fakeintake, or a hand-off to new-e2e. Finds the components the current diff touches, or asks which feature to test and maps the components it involves, then writes, wires and validates the test. Use when asked to "write a test", "add tests for my change", "test this feature", "cover this end to end", "make this an integration test", or "replace this e2e test with something cheaper". Not for writing or extending new-e2e tests the user already asked for by name (write-e2e), running existing tests (run-e2e), judging an existing suite only (e2e-audit), or Python tests.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Skill, AskUserQuestion
argument-hint: "[feature or behavior to test — omit to derive it from the current diff]"
---

Produce a test that catches the failures the change could introduce, at the cheapest level that still catches them. The usual mistake this skill prevents is jumping to the extremes: a unit test that mocks away the boundary that breaks, or a new-e2e test that provisions a VM for behavior an in-process integration test covers in seconds.

**Owning team:** `@DataDog/agent-devx`

**Prerequisites:** run from the repository root, with `dda` and Bazel set up (`dda inv install-tools`), `origin/main` fetched, Python 3.9 or later for `map_components.py`, and a `tar` accepting `--null -T -` (GNU tar or bsdtar) for `copy_worktree.sh`. Go tests only: Python checks and invoke tasks are out of scope.

## References

| File | Load when |
|---|---|
| `references/integration.md` | Step 3 picks an integration test — before writing it. It holds the wiring patterns and the pitfalls that make such tests weak or flaky. |
| `references/replacing-e2e.md` | The request is to replace an existing new-e2e suite with cheaper tests — in Step 0, before anything else. |
| `doc/guidelines/testing/test-categories.md` | You need the repository's definition of a test category to explain a verdict. |

## Step 0 — Establish the scope

When the request is to replace an existing new-e2e suite with cheaper tests, load `references/replacing-e2e.md` now and follow it: the suite's assertions are the behavior, and auditing the suite by its path, setup and provisioners included, replaces the discovery below and Step 2's audit of a described behavior. Step 1 still applies to the assertions moving to a cheaper test: map the hops each one needs from where its input enters, as the suite's setup and provisioner provide it, to where it is checked. That chain is the pipeline Step 3 wires and the report gives.

With an argument, treat it as the feature to test. Otherwise derive the scope from the change:

```bash
python3 -I .agents/skills/write-test/scripts/map_components.py
{ git diff --name-only "$(git merge-base HEAD origin/main)"; git ls-files --others --exclude-standard; } | grep -v '\.go$'
```

The script maps every Go file changed between the merge base with `origin/main` and the working tree, staged and unstaged edits included: untracked new files, which `git diff` alone would miss, and deleted files, whose component or importers are still entry points. A package deleted entirely, with nothing left importing it, is listed with the merge-base commands to read it. The second command lists the other changed files against the same base: a changed schema (`pkg/config/schema/yaml/`), profile or embedded YAML file names a behavior on its own, the effect of the setting or entry it changes, and its loader is the entry point for Step 1. When neither the Go files nor these identify a behavior, or the user wants to test something other than the diff, ask which feature or behavior to test with `AskUserQuestion`, offering the candidates you can see (recent commits, the files they mention). A behavior is something a user, another component, or Datadog can observe: a payload sent to intake, a status or diagnose output, a config key's effect, a CLI result. Restate it in that form before going on, because the rest of the procedure maps and tests *that* behavior, not the files.

## Step 1 — Map the components involved

List the components the behavior flows through, from where its input enters to where its output is observed. These are the candidates to wire for real in a test.

1. **Find the entry points.**
   - From the diff: take the changed Go files.
   - From a feature: find the code that implements it. Search for what a user would name: the config key (use the `locate-config-setting` skill for its schema node), the metric, tag, issue, or log message, the CLI subcommand, the intake endpoint. `git grep -n` the literal and keep the non-test hits.
2. **Map them to components:**
   ```bash
   python3 -I .agents/skills/write-test/scripts/map_components.py [<path>...]
   ```
   Without arguments it maps the Go files changed against `origin/main`. For each component or bundle reached, it prints the team, the directories that can build it in a test (`fx/`, `impl/`, `fx-mock/`, `mock/`, `fx-noop/`, `noop-impl/`, or `Bundle()`), and its consumers. A `pkg/` package is mapped to the components importing it; when none imports it directly, pass its importers to the script to follow the chain. Its importers outside `comp/` are listed after the table: a `cmd/` one is where a binary exposes the behavior, so keep it in the pipeline as an entry point. The consumers column lists the first 8 alphabetically; pass `--all` when you need them all, for example to find the bundle that wires a widely used component.
3. **Follow the pipeline.** The script maps Go code only: changed configuration such as a profile, a schema or an embedded YAML file (`defaultProfiles.yaml`, `pkg/config/schema/yaml/`) is a hop too, often the one that disagrees with the code, so add it to the chain. Read the `Requires` struct of each changed component's implementation (`impl/*.go`) for its upstream dependencies, and the consumers column for downstream ones. Stop at the boundary where the behavior becomes observable: a forwarder sending to intake, a status provider, a diagnose function, a file written. Read the code along the way rather than assuming the data path from names.
4. **Report the pipeline** before choosing a level, as a short chain with each hop's component and what it contributes, for example:

   > `datadog.yaml` + `DD_*` env → `comp/core/config` (loads, validates with `comp/core/schema`) → `comp/healthplatform` invalid-config check → store → forwarder → intake `/api/v2/agenthealth`

When the user named a feature and the mapping is ambiguous (several implementations, or a feature split across binaries), ask which path matters rather than testing all of them.

## Step 2 — Choose the level

Invoke the `e2e-audit` skill with the behavior and the pipeline from Step 1. It decides between a unit test, an integration test and new-e2e, by asking which boundary each assertion needs to keep real. When the request names a level ("write a unit test for this", "make this an integration test"), compare it with the audit before writing anything. When every assertion lands at that level, go on. When any lands elsewhere, lower or higher, show the split and ask one question: keep to the named level, with the other assertions under **Not covered** and the level they need, or write each at its own level. Write nothing until it is answered, the assertions at the named level included, since the answer can change the test that holds them. Then route each assertion in scope:

The audit classifies each assertion, and the assertions of one behavior can land at different levels. Split them:

- **Assertions needing E2E:** invoke `write-e2e` with only those assertions, each with the audit's reason and the hops it named as needing the deployed Agent. Pass the request's constraints as `write-e2e`'s flags, `--os`, `--env` and `--area`, whether they were flags or words: "cover this end to end on Windows" hands over `--os windows`, so the handoff can't fall back to a Linux suite. `write-e2e` gates its scope with its own `e2e-audit` run. When that run places an assertion lower, take its verdict as final and never hand it to `write-e2e` again, so the two skills can't send it back and forth. Write it here only when its new level is inside the scope already agreed: no level named, or the level the contributor agreed to. Otherwise list it under **Not covered** with that level, and ask before writing it.
- **Integration assertions:** load `references/integration.md` and continue with them. When the audit's boundary is a real local dependency (a daemon, a driver, hardware) rather than in-process components, follow its "Keep a real local dependency real" section.
- **Unit assertions:** continue with them.
- **Uncertain:** ask for the missing assertion or boundary the audit named, and audit again.

Wait for `write-e2e` to finish and keep its result: the assertions it covered and dropped (its `Scope`), the files it wrote, and its `Local run`, from which each handed-off assertion gets its status (see Output). Then stop only when no assertion is left in scope here: write the final report (see Output) from that result. An assertion `write-e2e` placed lower and that you write here (above) is in scope, like the lower-level assertions of the request.

A behavior often needs both: unit tests for the logic of the changed component, and one integration test proving the pipeline still carries it end to end. That pipeline test is the one most changes are missing — unit tests on each piece don't catch two pieces disagreeing (a schema rejecting what the loader produces, a payload field the forwarder drops). With no level named, write it like the other audited tests whenever the audit names an assertion whose boundary is between components. When the request named a level that excludes it, a unit test for example, propose it instead, under **Not covered**, and write it only when asked.

**Replacing an existing new-e2e test** follows `references/replacing-e2e.md`, loaded in Step 0: it says how to audit the suite, which assertions to move, and how to shrink the suite and its CI wiring without dropping coverage.

## Step 3 — Write the test

Read the existing tests next to the code first and follow their conventions: helpers, mocks, table layout, and build tags. The repository's test conventions win over this skill.

**Unit test**, in the package of the logic under test (`impl/` for a component):

- Table-driven, one case per behavior, named after the behavior.
- Mock only what the logic doesn't own, with the component's own `mock/` or `fx-mock/` package, or `config.NewMockWithOverrides` for settings.
- Assert on outputs and errors, not on calls to mocks, unless the call is the behavior.

**Integration test:** follow `references/integration.md`. In short: build the pipeline with the real `fx/` modules of every hop from Step 1, feed the input the way production does, capture the output at the observable boundary, and assert on all of the output the behavior's contract covers: a golden file when the whole payload is the contract, every series of a metric when it is one part of a shared payload.

For both, match the header and build tags of the package's other test files, and keep each test independent: no order dependency, no global state left behind.

## Step 4 — Wire it into the build

```bash
bazel run //:gazelle -- <package dir>
```

Run it after adding a test file or `testdata/` files, and after changing the imports of an existing test, which changes its target's `deps`. Keep the generated `BUILD.bazel` diff with the test changes: CI fails without it (`bazel:run-gazelle`). Committing is up to the contributor. Check that files the test reads at runtime are in the test target's `data` (gazelle adds `glob(["testdata/**"])`) and that `go:embed` files are in `embedsrcs`. Gazelle doesn't always remove entries for deleted files; remove those by hand. When a `BUILD.bazel` file changed, run `bazel run //bazel/buildifier:test`, and on failure `bazel run //bazel/buildifier` then the test again.

When the test adds a package, changes imports, or adds or changes a `//go:build` constraint, also run `dda inv go.write-tag-manifest`, and keep the generated `bazel/go_tags/closure_tags.bzl` diff with the test changes. A package missing from that manifest is built without the build tags it reads: a test file gated on `//go:build docker` compiles out, and the test passes without running. `bazel:run-gazelle` fails on a stale manifest too (see `bazel/AGENTS.md`).

## Step 5 — Validate

1. Run it, so that this invocation runs the test and prints its log, rather than reporting a cached result or leaving the log under a configuration directory that `bazel-testlogs` doesn't point to:
   ```bash
   dda inv test --targets=./<package> --test-args='-test.v' --bazel-args='--test_output=all --nocache_test_results'
   ```
   Read the subtest results in that output, not only the target status: a test that skips (for example, when a generated file is missing) passes too, and only `-test.v` prints `--- PASS` and `--- SKIP` per subtest. Add `-test.run='^TestX$/^<case>$'` to `--test-args` to iterate on one case: each segment is an unanchored regular expression otherwise, matching `TestXY` too. Run it again whenever the test changes: the last run on the contributor's tree is the one to report.
2. Prove it fails, by breaking the behavior once in a throwaway worktree holding a copy of the contributor's tree, uncommitted, staged and untracked edits included. The break never touches their files, so there is nothing to restore and nothing an interruption can leave behind in them.
   1. **Copy the tree:** `.agents/skills/write-test/scripts/copy_worktree.sh` checks out `HEAD` in a detached worktree under `.worktrees/`, applies the staged and unstaged changes as a patch (modes, binary files and deletions included), copies the untracked files that aren't ignored, and prints the copy's path. It writes nothing to the repository's object store or index, and removes the partial copy if a step fails. `.worktrees/` is ignored by git and excluded from Bazel's workspace. The copy has its own Bazel output base, so its first build takes longer, fed by the shared repository and remote caches. Ignored files aren't copied: when the test needs a generated file that is ignored, generate it in the copy with the same command. That includes `user.bazelrc`, the gitignored file of personal Bazel settings that `.bazelrc` imports from the workspace root, where `bazel/AGENTS.md` says to turn off the intranet-only ADMS dependency mirror. When the contributor's tree has one, copy it to the copy's root before the first run: Bazel in the copy reads the copy's own, and without it can fail to fetch dependencies off the intranet.
   2. **Check the copy passes first.** Run the filtered command below in the copy, unchanged: it must pass there. The copy has no ignored files, so a test failing in it before the break would fail after it for another reason. When it fails, generate what's missing in the copy and run it again; when it still fails, report the deliberate break as not run, with the failure.
   3. **Break and run.** Make one edit in the copy (an input fixture, a condition in the code), then run the Step 5.1 command from the copy's directory, filtered to the new test or subtest (`-test.run='^TestX$/^<case>$'` in `--test-args`), and record the assertion that failed. Check that the failure comes from that test, in that package: `--targets=./<package>` runs the packages below it too, so read the `--- FAIL: TestX/<case>` line and its target. Run unfiltered, an existing test of the package failing on the same break would hide whether the new one caught it. The test must fail with a message naming the problem: a test that still passes on the break was asserting on something else, and a skip is not a failure.
   4. **Remove the copy,** whatever the result: `git worktree remove --force <dir>`. A copy left by an interrupted run shows in `git worktree list` as a `.worktrees/write-test-*` entry, and is removed the same way.

   When the copy can't be made or the test can't run in it, report `Validated: deliberate break not run — <reason>`, and mark the test **written, not validated**, naming that check: **incomplete** is for an assertion with no test.
3. Lint, in the Go module holding the package: the linter defaults to the root module, and nested modules such as `test/new-e2e` or `pkg/util/log` have their own `go.mod`. Find it with `d=<package>; while [ ! -f "$d/go.mod" ] && [ "$d" != . ]; do d=$(dirname "$d"); done; echo "$d"`, then run `dda inv linter.go --module=<module> --targets=./<package relative to the module>` (`--module=.` for the root module). Typecheck other platforms when the test has platform-specific code (see the root `AGENTS.md`).

## Output

Report:

```markdown
**Behavior:** <the behavior, as observable>
**Pipeline:** <the chain from Step 1>
**Level:** <unit | integration | new-e2e> — <the e2e-audit reason, one line>

| Assertion | Test | Package | Real | Mocked, and why | Status |
| --- | --- | --- | --- | --- | --- |
| <the assertion> | `TestX/<case>` | `comp/...` | <components wired for real> | <dependency>: <outside the boundary because…> | <`validated` · **written, not validated**: <the check that didn't run or failed> · **incomplete**: <no test, why>> |

**Validated:** <the copy passing unchanged, then the deliberate break and the failure it produced there> · <the last run passing on the contributor's tree> · <lint result>
**Handed to write-e2e:** <each handed-off assertion with its status, read from `write-e2e`'s report: `validated` when its `Local run` shows every Step 9 gate passed with that test, **written, not validated** when it wrote the test but a gate didn't run or failed (name it), **incomplete** when it wrote no test or dropped the assertion (with its reason)> (omit when nothing was handed off)
**Not covered:** <boundaries this test doesn't preserve, if any, and whether another level covers them>
**Replaced:** <the suite, and for each of its assertions: moved to which test, or kept in E2E and why> · <the files, CI and JOBOWNERS entries removed or narrowed> · <the shrunk suite's lint and the run of its kept assertions, or not re-run> (only when replacing a new-e2e test)
```

Each assertion gets one row, so a report shows which passed every check when another couldn't be exercised. With assertions at several levels, give the Level and Validated lines once per test. When every assertion was handed off, the report keeps Behavior, Pipeline, a Level line per assertion, Handed to write-e2e, and Not covered; drop the table and the Validated line, since no test was written here, and give each assertion its status, with the same definitions as the Handed to write-e2e line: `validated`, **written, not validated**, or **incomplete** when no test was written.

The skill succeeded when every assertion has a test that was written and validated, here or by `write-e2e`; report an assertion handed to `write-e2e` with its status from that line: written, not validated, or incomplete, with the reason. For each test written here: it passed in the unchanged copy and failed on the deliberate break there, its last run on the contributor's tree passes, lint is clean, and every mock in the table has a reason that places it outside the behavior's boundary. An integration test asserts on everything the behavior's contract covers (see "Assert on what the contract covers" in the integration reference); for a unit test, every case of the table asserts on an output or an error. A replacement succeeded only when every assertion of the old suite is either covered by a validated test or still in E2E with a reason, every other suite the touched CI jobs ran before is still run by them, and nothing the removed tests used is left wired. When the suite was shrunk, its kept assertions must also lint and pass a run after the shrink; when it was removed entirely, there is nothing left to run, and the package still lints when other suites remain in it.

## Example

**Input:** "Test that invalid settings in `datadog.yaml`, `system-probe.yaml` or `DD_*` variables reach intake as health issues, without leaking their values."

**Report:**

```markdown
**Behavior:** an invalid setting in `datadog.yaml`, `system-probe.yaml` or a `DD_*` variable reaches intake as an agent health issue naming the setting, without its value
**Pipeline:** `datadog.yaml`, `system-probe.yaml` and `DD_*` env → `comp/core/config` and `comp/core/sysprobeconfig` (load, validate with `comp/core/schema`) → `comp/healthplatform` checks → store → forwarder → intake `/api/v2/agenthealth`
**Level:** integration — every hop runs in-process, and intake is replaced by a local fakeintake without losing the payload boundary

| Assertion | Test | Package | Real | Mocked, and why | Status |
| --- | --- | --- | --- | --- | --- |
| an invalid setting yields one issue naming it, its value absent, for each of the 9 scenarios | `TestInvalidConfigPayload/<scenario>` | `comp/healthplatform` | config and sysprobeconfig loading from files and `DD_*` variables, schema validation, healthplatform checks and store, forwarder | intake: an in-process fakeintake, which receives the same HTTP payload · secrets: the secrets mock, resolving `ENC[]` handles from `secrets.json`, since settings are validated before their handles are resolved and the backend binary is outside the payload boundary · log, telemetry, hostname, workloadmeta: their mocks, since no assertion reads them | `validated` |

**Validated:** making the invalid-setting check put the setting's value in the issue's message in the copy failed the `env_types` subtest on the golden comparison, whose diff showed the value · the 9 subtests pass on the contributor's tree · lint clean
**Not covered:** a real secret backend binary, the systemd environment file setting `DD_*` variables, and the packaged Agent's hostname and version. No other level covers them; keeping one new-e2e scenario as a smoke test would
```

Had the copy failed to build its scenarios for a missing ignored file, the row would read **written, not validated**: deliberate break not run, with that failure, and the report would say so rather than claim the test validated.

The first version of this test was a new-e2e suite provisioning an EC2 host per run. It became `comp/healthplatform/invalidconfig_payload_integration_test.go` ([#60753](https://github.com/DataDog/datadog-agent/pull/60753), not yet on `main` when this skill was written): the same 9 scenarios and golden reports, in about 4 seconds, failing on the same regressions.
