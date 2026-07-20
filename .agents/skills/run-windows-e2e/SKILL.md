---
name: run-windows-e2e
description: Run Windows E2E tests (MSI install tests or Fleet Automation/installer tests) locally against AWS-provisioned VMs
allowed-tools: Bash, Read, Glob, Grep, AskUserQuestion
argument-hint: "[suite] [TestFunctionName] [--build release|pipeline|local] [--pipeline-id <id>] [--version <version>]"
---

Run Windows E2E tests from `test/new-e2e/tests/windows/` or `test/new-e2e/tests/installer/windows/`.

Detailed reference material lives in `references/` next to this file — read the
relevant one when a step calls for it rather than duplicating it here:

- [`references/setup.md`](references/setup.md) — prerequisites, `~/.test_infra_config.yaml`, dev mode
- [`references/running.md`](references/running.md) — `setup-env`, local builds, `go test` flags
- [`references/vm-access.md`](references/vm-access.md) — connecting to a dev-mode VM (RDP/SSH)
- [`references/troubleshooting.md`](references/troubleshooting.md) — test outputs, Pulumi locks, AWS profile

## Instructions

### Step 1 — Parse `$ARGUMENTS`

Determine:
- **Suite**: which test suite to run (e.g. `install-test`, `service-test`, `agent-package`, `install-script`). If not provided, ask the user.
- **Test function**: specific `TestXxx` function. Most suites expect exactly one test per run — ask the user which one if not specified.
- **Artifact source**: `--build pipeline` (default), `--build local`, or `--build release`; pass `--pipeline-id <id>` through if given.
- **Stable/previous version** (upgrade tests only): if the user specifies a version to upgrade *from*, plan a second `setup-env` run with `--prefix STABLE_AGENT` in Step 3 (see [`references/running.md`](references/running.md) "Upgrade tests").
- **Branch**: if the user mentions a branch ("from main"), pass `--branch <name>` to `setup-env`. The default is the current git branch, which may have no pipelines if it's a local feature branch.

Map suite names to Go package paths:

| Suite | Package path |
|-------|-------------|
| `install-test` | `./test/new-e2e/tests/windows/install-test` |
| `service-test` | `./test/new-e2e/tests/windows/service-test` |
| `fips-test` | `./test/new-e2e/tests/windows/fips-test` |
| `domain-test` | `./test/new-e2e/tests/windows/domain-test` |
| installer / Fleet Automation (agent-package, install-script, install-exe, ddot, apm-inject, …) | `./test/new-e2e/tests/installer/windows` |

The installer / Fleet Automation tests are all one flat package (the
`suites/<package>/` subdirectories were flattened in #47161) — pick the area by
test function with `-run` (e.g. `TestAgentUpgrades`, `TestInstallScript`,
`TestDDOTExtensionViaMSI`, `TestAPMInjectInstalls`).

If the user gives a partial name or test function, search with Glob/Grep under `test/new-e2e/tests/windows/` and `test/new-e2e/tests/installer/windows/` to resolve it.

### Step 2 — Check prerequisites

```bash
test -f ~/.test_infra_config.yaml && echo "EXISTS" || echo "MISSING"
pulumi version 2>/dev/null || echo "MISSING"
```

If either is missing, offer to run `dda inv e2e.setup` and wait for the user to
complete its interactive prompts. If prerequisites exist but `devMode` is not
set, mention that `devMode: true` reuses VMs across runs (much faster for
iterative development). Full detail in [`references/setup.md`](references/setup.md).

### Step 3 — Resolve artifact environment variables

Run `setup-env` with `--fmt json` to capture the required env vars (no shell
`eval` needed — prepend the parsed pairs inline to `go test` in Step 5).

```bash
# From a pipeline (most common)
dda inv new-e2e-tests.setup-env --build pipeline --fmt json [--branch <branch>] [--pipeline-id <id>]

# From a local build (run `dda inv msi.build` first, + `msi.package-oci` for installer/OCI tests)
dda inv new-e2e-tests.setup-env --build local --fmt json
```

For upgrade tests, run a second time with `--prefix STABLE_AGENT` and merge the
vars in. GitLab token handling, local-build details, and the `STABLE_AGENT`
flow are in [`references/running.md`](references/running.md).

### Step 4 — Check for stale state (dev mode only)

If `devMode: true` and the user is rerunning, the previous VM may still have the
agent installed. Ask whether they've cleaned up (MSI tests: uninstall the agent;
installer tests: `datadog-installer.exe purge`). See
[`references/running.md`](references/running.md) "Clean state between runs".

### Step 5 — Build and confirm the `go test` command

```bash
go test -v -timeout 30m -tags test <package-path> -run <TestFunction>$
```

Two rules to apply (rationale in [`references/running.md`](references/running.md)):
anchor the `-run` regex with `$` at both suite and subtest level, and use the
exact package path with **no trailing `/...`** (or output won't stream).

Show the full command to the user and confirm before running.

### Step 6 — Run the test

Warn the user that AWS SSO auth may open a browser window when the test starts
(the test pauses until login completes), and that a non-sandbox `AWS_PROFILE`
will cause auth errors — advise `unset AWS_PROFILE`.

Run with `run_in_background: true` since tests provision real AWS VMs.
Provisioning takes a few minutes; once the VM is up, SSH becomes available
within ~60s (Linux) / ~180s (Windows). If SSH is not available within those
windows, troubleshoot before assuming the test is still running normally
(see [`references/vm-access.md`](references/vm-access.md)).

### Step 7 — Report results

When the test completes:
- Report pass/fail.
- On failure, point the user to `~/e2e-output/latest/` (crash dumps, agent/installer logs, event logs). If `devMode` is on, the VM is still up — offer to help RDP/SSH in via [`references/vm-access.md`](references/vm-access.md).
- For Pulumi lock errors or AWS auth errors, see [`references/troubleshooting.md`](references/troubleshooting.md).
