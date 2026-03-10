---
name: write-e2e
description: Write E2E tests for the Datadog Agent using the new-e2e framework with fakeintake assertions
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Agent
argument-hint: "<feature-or-check-name> [--platform linux|windows|both] [--env host|docker|k8s]"
---

Write end-to-end tests for the Datadog Agent using the `test/e2e-framework/` framework.
Parse `$ARGUMENTS` to determine what to test.

## Where to find what you need

| What | Where |
|------|-------|
| Framework API (environments, provisioners, agentparams) | `test/e2e-framework/AGENTS.md` |
| Fakeintake API (payload types, client methods, extending) | `test/fakeintake/AGENTS.md` |
| Setup, prerequisites, running tests | `docs/public/how-to/test/e2e.md` |
| Real tests to use as patterns | `test/new-e2e/tests/` (see lookup table in e2e-framework AGENTS.md) |
| Check system / Python check conventions | root `AGENTS.md` § "Check System" |
| Test placement / team ownership | `CODEOWNERS` |
| CI job definitions | `.gitlab/test/e2e/e2e.yml`, `.gitlab/windows/test/e2e/windows.yml` |
| CI trigger rules | `.gitlab-ci.yml` (search for `.on_*_or_e2e_changes`) |

Read the first two files before writing any test. Browse a few real tests
that match your use case.

## Checklist

1. Read the feature's implementation to understand what payloads it sends
2. Check if E2E tests already exist under `test/new-e2e/tests/`
3. Place tests in the right `<area>` directory (check `CODEOWNERS`);
   one file per platform target (e.g., `disk_nix_test.go`, `disk_win_test.go`)
4. Use `require` (not `assert`) inside `EventuallyWithT` callbacks
5. Verify compilation: `cd test/new-e2e && go vet ./tests/<area>/...`
6. **Run the test locally before pushing** — compilation alone is not enough:
   `dda inv new-e2e-tests.run --targets=./tests/<area>/...`
   See `test/e2e-framework/AGENTS.md` § "Validating and troubleshooting" if it fails
7. Check CI wiring: `grep -n 'TARGETS:.*<area>' .gitlab/test/e2e/e2e.yml`

## Output

Show the user: files created, how to compile-check, how to run locally
(`dda inv new-e2e-tests.run --targets=./tests/<area>/...`), and whether
CI changes are needed.
