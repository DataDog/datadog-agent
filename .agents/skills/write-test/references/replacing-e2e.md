# Replacing an existing new-e2e test

When the request is to replace a suite with something cheaper, its assertions are the behavior. The procedure moves the assertions a cheaper test can hold, and keeps the others in the suite, with their CI coverage intact.

## Audit the suite by its path

Invoke `e2e-audit` with the suite's concrete path rather than a description of it: its procedure for existing tests follows the suite's setup, helpers and provisioners, which hold coverage the test bodies don't show (a service restart in setup, an install in the provisioner), and returns a verdict per assertion. Record, with each verdict, the boundary it rests on: the E2E boundary an assertion staying in the suite needs, and for an assertion moving to a cheaper test, the hops its pipeline must keep, which Step 1 maps. Split the verdicts as Step 2 of the skill does, with one difference: an assertion that still needs E2E stays in the suite as it is, and goes to `write-e2e` only when the audit says the suite lacks coverage it needs.

## Move only what a validated test replaces

Write and validate the lower-level tests (Steps 3 to 5). Remove an assertion from the suite only once the test replacing it is validated and a CI job runs it: name the job. `dda inv test` leaves out the targets tagged for a local dependency (`tagset_bpf`, `tagset_functionaltests`, and the others in `bazel/configs/go_tests.bazelrc`), so a replacement validated locally with a tag override runs in CI only in a job selecting that tag, on a runner providing the dependency. Until one does, keep the assertion in E2E and report why. Keep in the suite any assertion still uncertain, or whose replacement isn't validated, and report it as kept with the reason. Ask before removing a suite another team owns (`.github/CODEOWNERS`).

## Shrink the suite and its wiring

Shrink the suite to the assertions that still need E2E, or remove it when none do. Remove what only served the removed tests too: fixtures, helpers, then rerun gazelle.

CI selects suites by package and by name: a job's `TARGETS` lists packages, and its `EXTRA_PARAMS` (`--run TestECSSuite`, `--skip ...`), or the template it extends, narrows them to some suites. A `parallel: matrix:` job sets `EXTRA_PARAMS` per entry, each entry a CI run of its own, and several jobs can share one package: `new-e2e-containers-ecs` and `new-e2e-containers` both target `./tests/containers`, and `new-e2e-containers` has six entries running only `TestKindSuite` beside entries running `TestDockerSuite` and the rest. Find every job running the package, not only those naming it: a `TARGETS` can name the package itself (`./tests/installer/script`), or any of its parents with or without `/...` (`./tests/installer`, `./tests/...`), and a job can inherit `TARGETS` or `EXTRA_PARAMS` from a template through `extends:`. Grep the package and each of its ancestors, and resolve each job's `extends:` chain to the values it actually runs:

```bash
p=tests/<package path>  # relative to test/new-e2e, e.g. tests/installer/script
while [ "$p" != . ]; do grep -rn -E "TARGETS:.*\./$p(/\.\.\.)?( |\$|,|\")" .gitlab/; p=$(dirname "$p"); done
```

Before changing anything, list for each of those jobs and matrix entries what it runs, in every package its `TARGETS` covers: the suites, and the methods its filter selects when it names some (`--run TestAgentUpgrades$/TestUpgradeMSI$` in the Windows installer matrix runs one method of a suite, not the suite). A list of suites alone misses an entry whose suite is kept but whose method is removed: it still lists the suite, and runs no assertion. Then evaluate each matrix entry, or the job's own `EXTRA_PARAMS` when it has no matrix:

- **An entry selecting only the removed suite, or only removed methods:** remove the entry.
- **An entry selecting it with others through an explicit `--run` naming it:** remove its name from that filter and leave the rest as it is. Leave a `--skip` naming it as it is too, since it skips nothing once the suite is gone.
- **An entry selecting it with others implicitly**, with no `--run`, or a pattern matching more than its name: leave it alone. Removing the suite from the package already removes it from the entry, and a filter listing the suites left would silently exclude the suites added after them.
- **An entry not selecting it:** leave it alone.

Then clean up each job by its kind:

- **A matrix job:** when every entry is removed, remove the job and its `.gitlab/JOBOWNERS` entry, whatever else its `TARGETS` lists: a `parallel: matrix:` with no entry is invalid, and each removed entry selected only removed assertions, as the coverage check below confirms.
- **A standalone job:** when no suite it selects is left in the package, remove the package from its `TARGETS`, and the job and its `.gitlab/JOBOWNERS` entry when no target is left. Never narrow a `TARGETS` naming a parent directory, or a template other jobs extend: the other packages and jobs rely on it. Last, list what each job and entry runs again: every suite and method of the first list, other than the removed ones, must still be in it, and every entry left must select at least one test. An entry whose filter named only removed methods is removed like one selecting only the removed suite.

## Check what's left

Removing a test, a helper or a fixture can break the shared setup a kept assertion relies on. Lint the shrunk suite (`dda inv linter.go --module=test/new-e2e --targets=./tests/<area>`), and run its kept assertions with `write-e2e`'s Step 9 G4, which provisions cloud infrastructure: show the commands and get agreement first. When that run doesn't happen, report the kept assertions as **not re-run**.

Report what became of each assertion under **Replaced** (see the skill's Output).

## Example

**Input:** "Replace the agent-health e2e suite checking invalid-config payloads with something cheaper." This is the first version of the skill's main example, before [#60753](https://github.com/DataDog/datadog-agent/pull/60753) replaced it; the suite and file names are illustrative.

The audit, run on the suite's file under `test/new-e2e/tests/agent-health/`, classified the 9 payload scenarios as integration: every hop runs in-process, and a local fakeintake receives the same payload. It kept one assertion in E2E: that the packaged Agent, started by systemd with `DD_*` variables from its environment file, reports the issue, since the service manager is the boundary.

```markdown
**Replaced:** `TestInvalidConfigSuite` — the 9 payload scenarios moved to `TestInvalidConfigPayload/<scenario>` in `comp/healthplatform`, each validated · the systemd environment-file scenario kept in E2E: the service manager setting `DD_*` variables is the boundary · removed the 9 scenario methods and their fixtures; `new-e2e-agent-health` still runs the package's other suites and this one, so its `TARGETS` and JOBOWNERS entry stay · shrunk suite: lint clean, the kept scenario passed G4
```
