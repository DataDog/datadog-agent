---
name: write-e2e
description: Write E2E tests for the Datadog Agent using the new-e2e framework with fakeintake assertions
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Agent
argument-hint: "<feature-or-check-name> [--platform linux|windows|both] [--env host|docker|k8s]"
---

Write end-to-end tests for the Datadog Agent. Parse `$ARGUMENTS` to
determine what to test.

## Where to find what you need

| What | Where |
|------|-------|
| Framework API (environments, provisioners, agentparams) | `test/e2e-framework/AGENTS.md` |
| Fakeintake API (payload types, client methods, extending) | `test/fakeintake/AGENTS.md` |
| Setup, prerequisites, running tests | `docs/public/how-to/test/e2e.md` |
| Real tests to use as patterns | `test/new-e2e/tests/` (see lookup table in e2e-framework AGENTS.md) |
| Test placement / team ownership | `CODEOWNERS` |
| CI job definitions | `.gitlab/test/e2e/e2e.yml`, `.gitlab/windows/test/e2e/windows.yml` |
| CI trigger rules | `.gitlab-ci.yml` (search for `.on_*_or_e2e_changes`) |

Read the first two files before writing any test. Browse a few real tests
that match your use case.

## Things to get right

- **Research first**: read the feature's implementation and unit tests to
  understand what payloads it sends (not just metrics — could be logs, events,
  traces, SBOMs, container images, etc.)
- **Check existing coverage**: E2E tests may already exist under `test/new-e2e/tests/`
- **Place tests correctly**: check `CODEOWNERS` for the right `<area>` directory
- **License header**: every test file needs the Apache 2.0 header
- **Assertions**: use `require` (not `assert`) inside `EventuallyWithT` callbacks;
  2 min timeout, 10s interval is the default
- **Verify compilation**: `cd test/new-e2e && go vet ./tests/<area>/...` —
  do NOT run the test (it provisions real cloud infrastructure)
- **CI wiring**: check if an existing job already covers your test directory
  (`grep -n 'TARGETS:.*<area>' .gitlab/test/e2e/e2e.yml`). If not, add trigger
  rules and a job definition — look at existing jobs for the pattern
- **Run locally**: `dda inv new-e2e-tests.run --targets=./tests/<area>/...`
  (handles AWS auth internally, no `aws-vault exec` wrapper needed)

## Output

Show the user: files created, how to compile-check, how to run locally
(`/run-e2e`), and whether CI changes are needed.

## Keeping this skill accurate

Part of the `AGENTS.md` hierarchy (see root `AGENTS.md` § "Keeping AI
context accurate"). Fix this file and the relevant `AGENTS.md` files if
you find inaccuracies.
