---
name: write-agent-health-e2e
description: Create an E2E lifecycle test for a new health platform issue in test/new-e2e/tests/agent-health/
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
argument-hint: "<issue-module-name>  e.g. rofspermissions, invalidconfig, dockerpermissions"
---

Create an E2E lifecycle test for a health platform issue module.
Parse `$ARGUMENTS` for the module name (matches a directory under `comp/healthplatform/issues/`).

## Step 1 — Read the issue module

Read these two files before writing anything:

- `comp/healthplatform/issues/{name}/module.go` — find `IssueID`, `IssueName`, and whether the module has:
  - `BuiltInPeriodicHealthCheck()` returning non-nil → the agent runs the check itself on a schedule
  - `BuiltInStartupHealthCheck()` returning non-nil → the check runs once at startup
  - Both returning `nil` → issues are pushed externally by the collector (like `checkfailure`)
- `comp/healthplatform/issues/{name}/issue.go` — extract `Category`, `Source`, `Tags`, and `Remediation` fields to assert in the test

## Step 2 — Choose the environment

| Scenario | Environment |
|---|---|
| Standard host issue (most cases) | `environments.Host` + `awshost.Provisioner` |
| Issue requires Docker daemon | Custom env struct with `Docker *components.RemoteHostDocker` + Pulumi provisioner |

Do NOT implement `Diagnose()` on the env struct — all assertions go through fakeintake.

For the standard case the provisioner looks like:
```go
e2e.WithProvisioner(awshost.Provisioner(
    awshost.WithRunOptions(
        ec2.WithAgentOptions(
            agentparams.WithAgentConfig(healthPlatformAgentConfig),
            // add agentparams to trigger the issue if needed
        ),
    ),
))
```

`healthPlatformAgentConfig` is defined in `check_failure_test.go` (same package):
```go
const healthPlatformAgentConfig = `health_platform:
  enabled: true
  forwarder:
    interval: 30s
`
```

## Step 3 — Write the test file

File: `test/new-e2e/tests/agent-health/{name}_test.go`

### Exact structure to follow

```go
type {name}Suite struct {
    e2e.BaseSuite[environments.Host]
}

func Test{Name}Suite(t *testing.T) {
    e2e.Run(t, &{name}Suite{},
        e2e.WithProvisioner(awshost.Provisioner(...)),
    )
}

// Test{Name}IssueLifecycle verifies ...
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *{name}Suite) Test{Name}IssueLifecycle() {
    fakeIntake := suite.Env().FakeIntake.Client()
    // only declare host/agent if needed for trigger/fix actions

    const issueID = "..." // exact IssueID from module.go

    suite.T().Run("IssueDetection", func(t *testing.T) {
        // trigger the issue if not pre-configured by the provisioner
        var issues []*healthplatform.Issue
        require.EventuallyWithT(t, func(ct *assert.CollectT) {
            payloads, err := fakeIntake.GetAgentHealth()
            assert.NoError(ct, err)
            issues = nil
            for _, p := range payloads {
                for _, iss := range findIssuesByID(t, p, issueID) {
                    if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW {
                        issues = append(issues, iss)
                    }
                }
            }
            assert.NotEmpty(ct, issues, "issue not found as NEW in fakeintake")
        }, defaultIssueTimeout, defaultIssuePollInterval, "issue not detected as NEW")

        require.NotEmpty(t, issues)
        issue := issues[0]
        // assert fields from issue.go: IssueName, Category, Source, Tags, Remediation
    })

    suite.T().Run("Resolution", func(t *testing.T) {
        // apply the fix: either suite.UpdateEnv(...) or host.MustExecute(...)

        // FLUSH ORDER: restart/UpdateEnv FIRST, wait for ready, THEN flush
        // With UpdateEnv (handles restart + wait internally):
        suite.UpdateEnv(awshost.Provisioner(...)) // deploy fix
        require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

        // With manual restart:
        // require.NoError(t, agent.Client.Restart())
        // require.EventuallyWithT(t, func(ct *assert.CollectT) {
        //     assert.True(ct, agent.Client.IsReady())
        // }, 2*time.Minute, 10*time.Second, "agent not ready")
        // require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

        require.Never(t, func() bool {
            payloads, _ := fakeIntake.GetAgentHealth()
            for _, p := range payloads {
                for _, iss := range findIssuesByID(t, p, issueID) {
                    if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
                        return true
                    }
                }
            }
            return false
        }, defaultIssueAbsenceWindow, defaultIssuePollInterval,
            "issue still reported as non-resolved after fix")
    })
}
```

Use `findIssuesByPrefix(p, prefix)` instead of `findIssuesByID` when the issue ID includes a runtime-generated suffix (e.g. `check-execution-failure:broken_check:<hash>`). Append `*` to `issueID` as a reminder that prefix matching is needed.

## Step 4 — Fixture rules

| Content | How to include |
|---|---|
| Short YAML (≤ 10 lines) | `const` string literal inline in the test file |
| Python check file | `//go:embed fixtures/name.py` + `var nameContent string` |
| Long YAML or binary | `//go:embed fixtures/name.yaml` + `var nameContent string` |

Never embed YAML that is already defined as a `const` in another file in the same package.

## Step 5 — What NOT to add

- No `Diagnose()` method on the env struct
- No `RunHealthIssueLifecycle` or other shared lifecycle driver
- No `AssertIssueDetectedViaDiagnose` or any diagnose-based assertion
- No `RestartResilience` phase — covered once by `TestResilienceSuite`
- No `agent.Client.IsReady()` wait at suite start — the framework guarantees this
- No flush before restart — always restart (or UpdateEnv) first, wait for ready, then flush

## Step 6 — Verify

```bash
cd test/new-e2e && go vet ./tests/agent-health/...
```

Remind the user to run the test locally before pushing:
```bash
dda inv new-e2e-tests.run --targets=./tests/agent-health/...
```
