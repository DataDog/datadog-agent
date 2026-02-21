---
name: run-e2e
description: Run E2E tests locally using the new-e2e framework with Pulumi-based infrastructure
allowed-tools: Bash, Read, Glob, Grep
argument-hint: "<test-path-or-name> [--run TestName] [--keep-stack] [--configparams key=value]"
---

Run E2E tests from `test/new-e2e/tests/` using `dda inv new-e2e-tests.run`.

## Instructions

1. **Parse `$ARGUMENTS`** to determine what to run. The user may provide:
   - A test directory path (e.g., `windows/install-test`, `agent-platform/upgrade`, `containers`)
   - A test function name (e.g., `TestInstall`, `TestUpgrade`)
   - Flags to pass through (see below)
   - A combination of the above

2. **Resolve the test target**:
   - The invoke task automatically prepends `test/new-e2e/` to targets, so targets must be relative to that directory (e.g., `./tests/agent-subcommands/flare`, NOT `./test/new-e2e/tests/...`)
   - If the user gives a directory like `agent-subcommands/flare`, use `./tests/agent-subcommands/flare` as `--targets`
   - If the user gives a partial name, search for matching directories under `test/new-e2e/tests/` using Glob, then strip the `test/new-e2e/` prefix for the target
   - If the user gives a test function name (starts with `Test`), find which package contains it using Grep under `test/new-e2e/tests/`, then set `--targets` to the package path (relative to `test/new-e2e/`) and `--run` to the test name
   - If ambiguous, list the matching options and ask the user to pick one

3. **Ask about keeping infrastructure** (if `--keep-stack` not already in `$ARGUMENTS`):
   Use `AskUserQuestion` to ask the user if they want to keep the test infrastructure running:

   **Question**: "Do you want to keep the test infrastructure running after the test completes?"

   **Options**:
   - "Clean up automatically (Recommended)" — Destroy all test infrastructure after the test finishes. This is the default and prevents costs from accumulating.
   - "Keep infrastructure running" — Leave all resources up after the test. Useful for debugging, but requires manual cleanup later to avoid charges.

   If the user chooses to keep infrastructure, add `--keep-stack` to the command.

4. **Build the command**:
   ```
   dda inv new-e2e-tests.run --targets=./tests/<path> [flags]
   ```
   IMPORTANT: `--targets` paths are relative to `test/new-e2e/`. Do NOT include `test/new-e2e/` in the target path.

5. **Supported flags** (pass through from `$ARGUMENTS`):
   - `--run <regex>` — Only run tests matching this regex
   - `--skip <regex>` — Skip tests matching this regex
   - `--keep-stack` — Keep infrastructure up after test (for debugging)
   - `--configparams <key=value>` — Override Pulumi ConfigMap parameters
   - `--agent-image <image:tag>` — Use a specific agent image
   - `--cluster-agent-image <image:tag>` — Use a specific cluster agent image
   - `--stack-name-suffix <suffix>` — Add suffix to stack name (useful for stuck stacks)
   - `--verbose` / `--no-verbose` — Toggle verbose output (default: verbose)
   - `--max-retries <n>` — Retry failed tests up to n times
   - `--flavor <flavor>` — Package flavor (e.g., "datadog-agent")
   - `--cache` — Enable test cache (disabled by default)

6. **Before running**, confirm the full command with the user.

7. **Run the command** with a 60-minute timeout (infrastructure provisioning can take a while). Use `run_in_background` for the Bash tool since e2e tests are long-running.

8. **After completion**, summarize the results: which tests passed, which failed, and any useful error output.

## Prerequisites

The following must be configured before running e2e tests:
- `pulumi` CLI installed
- `~/.test_infra_config.yaml` exists with proper configuration

If any prerequisite is missing, inform the user what needs to be set up.

## Available Test Suites

Tests are organized under `test/new-e2e/tests/`, running `ls test/new-e2e/tests` should give the list of test packages


# Issues when running tests
If the test was previously executed and that the infra is in a weird state, that Pulumi is not aware of, trying to rerun the test with the same stack can lead to strange error, like resource being replaced, while they should not exist at all yet.
To avoid that issue it is possible to execute the test with a stack with a different name, using --stack-name-suffix <suffix>, please use short stack name suffix.
Stop the execution early when you detect that issue in the logs

## Examples

```bash
# Run all tests in a directory
dda inv new-e2e-tests.run --targets=./tests/windows/install-test

# Run a specific test
dda inv new-e2e-tests.run --targets=./tests/agent-platform/upgrade --run TestUpgrade

# Keep stack for debugging
dda inv new-e2e-tests.run --targets=./tests/containers --run TestContainerLinux --keep-stack

# Run with specific agent image
dda inv new-e2e-tests.run --targets=./tests/agent-platform --agent-image "my-registry/agent:latest"

# Run with stack name suffix
dda inv new-e2e-tests.run --targets=./tests/windows/install-test --stack-name-suffix 2



## Usage

- `/run-e2e windows/install-test` — Run all Windows install tests
- `/run-e2e windows/install-test --run TestInstall` — Run only TestInstall
- `/run-e2e TestUpgrade` — Auto-find and run TestUpgrade
- `/run-e2e agent-platform --keep-stack` — Run with stack kept alive

## Output

Show the user the full command before running, then report test results when done.
