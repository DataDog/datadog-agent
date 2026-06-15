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
| Framework API (environments, provisioners, agentparams, installers) | `test/e2e-framework/AGENTS.md` |
| Fakeintake API (payload types, client methods, extending) | `test/fakeintake/AGENTS.md` |
| Setup, prerequisites, running tests | `docs/public/how-to/test/e2e.md` |
| Real tests to use as patterns | `test/new-e2e/tests/` |
| Test tier guidance (T0–T4) | `test/new-e2e/AGENTS.md` |
| Check system overview | root `AGENTS.md` § "Check System" |
| Test placement / team ownership | `CODEOWNERS` |
| Examples | `test/new-e2e/examples/` (Pattern A = canonical, Pattern B = explicit) |

Read the first two files before writing any test.

## Writing tests — key rules

**Agent installation** happens automatically via PostProvision when you pass
`WithAgentOptions(...)` to a provisioner. Do NOT call `hostagent.Install` or
`helmagent.Install` in `SetupSuite` unless you have a bespoke install requirement.

**Pattern A (canonical)** — provisioner handles everything:
```go
func TestMyFeature(t *testing.T) {
    e2e.Run(t, &mySuite{}, e2e.WithProvisioner(
        awshost.Provisioner(awshost.WithRunOptions(
            ec2.WithAgentOptions(agentparams.WithAgentConfig("log_level: debug")),
        )),
    ))
}
// No SetupSuite override needed — agent is installed in PostProvision
```

**Agent reconfiguration** (mid-test) — use `Configure`, NOT `UpdateEnv`:
```go
func (s *mySuite) TestReconfigure() {
    // Fast: rewrites config + restarts agent via SSH, no Pulumi cycle
    s.Env().Agent.Configure(s.T(), agentparams.WithAgentConfig("log_level: info"))
}
```

**When to use `UpdateEnv`**: only when you need to change INFRASTRUCTURE (different
OS, different node group, etc.), not just agent config. Agent-config-only changes are
faster with `Agent.Configure`.

## Checklist

1. Read the feature's implementation to understand what payloads it sends
2. Check if E2E tests already exist under `test/new-e2e/tests/`
3. Place tests in the right `<area>` directory (check `CODEOWNERS`);
   one file per platform target (e.g., `disk_nix_test.go`, `disk_win_test.go`)
4. Verify compilation: `cd test/new-e2e && go vet ./tests/<area>/...`
5. CI runs E2E tests — tests cannot be run locally. Verify CI wiring:
   `grep -n 'TARGETS:.*<area>' .gitlab/test/e2e/e2e.yml`

## Output

Show the user: files created, how to compile-check, and whether CI changes are needed.
