---
name: write-e2e
description: Write E2E tests for the Datadog Agent using the new-e2e framework with fakeintake assertions
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Agent
argument-hint: "<feature-or-check-name> [--platform linux|windows|both] [--env host|docker|k8s]"
---

Write end-to-end tests for the Datadog Agent using the `test/new-e2e/` framework.

## Before you start — read the reference docs

These files are the source of truth. Read them before writing any test:

- **`test/e2e-framework/AGENTS.md`** — environments, provisioners, agentparams, key files
- **`test/fakeintake/AGENTS.md`** — all supported endpoints/payload types, client API, how to extend
- **`docs/public/how-to/test/e2e.md`** — setup, running tests, dev mode
- **`test/new-e2e/tests/`** — real tests to use as patterns (browse a few before writing)

## Workflow

### 1. Understand the feature

Parse `$ARGUMENTS` to determine what to test. Then **research**:
- Read the implementation in `pkg/` or `comp/` to understand what data it produces
- Read existing unit tests to understand edge cases
- Check if E2E tests already exist under `test/new-e2e/tests/`
- Identify **all expected payloads** the feature sends to the intake — see
  `test/fakeintake/AGENTS.md` for the full list (metrics, logs, events, traces,
  service checks, processes, containers, SBOMs, orchestrator resources, etc.)

### 2. Choose environment and placement

Pick the environment from `test/e2e-framework/AGENTS.md` (Host, DockerHost,
Kubernetes, ECS). Default to `environments.Host` for most checks.

Place tests under `test/new-e2e/tests/<area>/` — check `CODEOWNERS` to find the
right `<area>` for the team that owns the feature.

### 3. Write the test

**File layout:**
```
test/new-e2e/tests/<area>/
├── <name>_test.go           # or split into _common/_nix/_win for platform tests
└── fixtures/                # embedded config files if needed
```

**Key patterns** (see real tests for examples):
- Embed `e2e.BaseSuite[environments.Host]` in your suite struct
- Use `e2e.Run(t, suite, e2e.WithProvisioner(...))` as entry point
- Call `t.Parallel()` in the top-level `TestXxx` function
- Configure the agent with `agentparams` options (see `test/e2e-framework/AGENTS.md`)
- Assert with `s.EventuallyWithT(func(c *assert.CollectT) { ... }, 2*time.Minute, 10*time.Second)`
- Use `require` (not `assert`) inside `EventuallyWithT` callbacks
- Use fakeintake client methods to filter payloads (see `test/fakeintake/AGENTS.md`)

**Every test file must start with the Apache 2.0 license header.**

### 4. Verify compilation

```bash
cd test/new-e2e && go vet ./tests/<area>/...
```

Do NOT run the test — it provisions real cloud infrastructure.

### 5. Wire into CI

E2E tests must run automatically when related source code changes.

**Check if an existing job covers your test directory:**
```bash
grep -n 'TARGETS:.*<area>' .gitlab/test/e2e/e2e.yml .gitlab/windows/test/e2e/windows.yml
```

If the test lives under an existing `TARGETS` directory, the job already picks
it up. If not:

**A. Trigger rules** (`.gitlab-ci.yml`) — add source paths that should trigger the test:
```yaml
.on_<feature>_or_e2e_changes:
  - !reference [.on_e2e_main_release_or_rc]
  - changes:
      paths:
        - pkg/collector/corechecks/<feature>/**/*
        - test/new-e2e/tests/<area>/**/*
      compare_to: $COMPARE_TO_BRANCH
```

**B. Job definition** (`.gitlab/test/e2e/e2e.yml`) — add a job if needed:
```yaml
new-e2e-<feature>:
  extends: .new_e2e_template_needs_deb_x64
  rules:
    - !reference [.on_<feature>_or_e2e_changes]
    - !reference [.manual]
  variables:
    TARGETS: ./tests/<feature>
    TEAM: <team-name>
    EXTRA_PARAMS: --skip "Windows"
```

### 6. Check if framework/fakeintake changes are needed

If the feature sends a payload type not yet supported by fakeintake, or needs a
provisioner option that doesn't exist, you'll need to extend them. See the
`AGENTS.md` in each sub-project for how.

## Running locally

```bash
dda inv new-e2e-tests.run --targets=./tests/<area>/...
```

The invoke task handles AWS authentication internally — no `aws-vault exec`
wrapper needed. See `docs/public/how-to/test/e2e.md` for prerequisites.

## Output

Show the user:
1. The files created/modified
2. How to compile-check the test
3. How to run it locally with `/run-e2e`
4. Whether CI integration is needed

## Keeping this skill accurate

This skill is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). If you discover that a step is wrong or
incomplete, update this file and the relevant `AGENTS.md` files.
