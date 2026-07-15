---
name: write-agent-health-e2e
description: Create an E2E lifecycle test for a new health platform issue in test/new-e2e/tests/agent-health/
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
argument-hint: "<issue-module-name>  e.g. rofspermissions, invalidconfig, dockerpermissions"
---

## Parse the argument

`$ARGUMENTS` must be the name of an existing issue module directory under `comp/healthplatform/issues/`.

**If `$ARGUMENTS` is empty or not provided, stop immediately and ask:**
> Which health platform issue module should I write the E2E test for?
> Available modules: run `ls comp/healthplatform/issues/` to list them.

Once you have the module name, set:
- `MODULE` = `$ARGUMENTS` (e.g. `rofspermissions`)
- `MODULE_PATH` = `comp/healthplatform/issues/$MODULE/`
- `TEST_FILE` = `test/new-e2e/tests/agent-health/{module}_test.go`

Verify `$MODULE_PATH` exists before proceeding:
```bash
ls comp/healthplatform/issues/$MODULE/
```
If the directory does not exist, stop and tell the user which modules are available.

---

## Step 1 — Read the issue module

Read both files before writing anything:

**`$MODULE_PATH/module.go`** — extract:
- `IssueID` constant → used as `const issueID` in the test
- `IssueName` constant → asserted as `issue.IssueName` in the test
- Whether `BuiltInPeriodicHealthCheck()` returns non-nil → agent runs the check on a schedule
- Whether `BuiltInStartupHealthCheck()` returns non-nil → check runs once at startup
- Both returning `nil` → issues are pushed externally by another component (like `admissionprobe`)

**`$MODULE_PATH/issue.go`** — extract the exact values returned by `BuildIssue()`:
- `Category`, `Source`, `Location`, `Severity`, `Tags` → assert these in `IssueDetection`
- Whether `Remediation` is non-nil and has `Summary`/`Steps` → assert those too

If a `check.go` file exists in the module, read it to understand how the issue is triggered (what system condition causes it).

---

## Step 2 — Choose the environment

| Scenario | Environment |
|---|---|
| Standard host issue (most cases) | `environments.Host` + `awshost.Provisioner` |
| Issue requires Docker daemon | Custom env struct with `Docker *components.RemoteHostDocker` + Pulumi provisioner |

Do **not** implement `Diagnose()` on the env — all assertions go through fakeintake only.

Check whether another test file in this package already embeds a suitable base agent config (e.g. a short forwarder interval) before adding your own — reuse it if so, following the fixture rules in Step 4 otherwise.

---

## Step 3 — Write `$TEST_FILE`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/DataDog/agent-payload/v5/healthplatform"

    "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
    "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
    awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type {module}Suite struct {
    e2e.BaseSuite[environments.Host]
}

func Test{Module}Suite(t *testing.T) {
    t.Parallel()
    e2e.Run(t, &{module}Suite{},
        e2e.WithProvisioner(awshost.Provisioner(
            awshost.WithRunOptions(
                ec2.WithAgentOptions(
                    agentparams.WithAgentConfig({module}AgentConfig),
                    // add agentparams here to pre-configure the issue trigger if needed
                ),
            ),
        )),
    )
}

// Test{Module}IssueLifecycle verifies that <describe trigger condition> is detected
// in fakeintake as NEW, and that <describe fix> causes the issue to stop being
// reported (or be reported as RESOLVED).
//
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *{module}Suite) Test{Module}IssueLifecycle() {
    // only declare host/agent if needed for runtime trigger/fix steps
    fakeIntake := suite.Env().FakeIntake.Client()

    const issueID = "<IssueID from module.go>"

    suite.T().Run("IssueDetection", func(t *testing.T) {
        // if the issue is not pre-configured by the provisioner, trigger it here:
        // suite.Env().RemoteHost.MustExecute("sudo ...")

        var issues []*healthplatform.Issue
        require.EventuallyWithT(t, func(ct *assert.CollectT) {
            payloads, err := fakeIntake.GetAgentHealth()
            assert.NoError(ct, err)
            issues = nil
            for _, p := range payloads {
                for _, iss := range findIssuesByID(t, p, issueID) {
                    if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
                        issues = append(issues, iss)
                    }
                }
            }
            assert.NotEmpty(ct, issues, "issue not found as ACTIVE in fakeintake")
        }, defaultIssueTimeout, defaultIssuePollInterval, "issue not detected as ACTIVE in fakeintake")

        require.NotEmpty(t, issues)
        issue := issues[0]
        // assert values read from issue.go:
        assert.Equal(t, "<IssueName>", issue.IssueName)
        assert.Equal(t, "<Category>", issue.Category)
        assert.Equal(t, "<Source>", issue.Source)
        // assert.Equal(t, "<Location>", issue.Location)  // if Location is set
        assert.Contains(t, issue.Tags, "<tag>")
        require.NotNil(t, issue.Remediation)
        assert.NotEmpty(t, issue.Remediation.Summary)
    })

    suite.T().Run("Resolution", func(t *testing.T) {
        // Option A — fix via config change (preferred): UpdateEnv handles restart + wait
        suite.UpdateEnv(awshost.Provisioner(
            awshost.WithRunOptions(
                ec2.WithAgentOptions(
                    agentparams.WithAgentConfig({module}AgentConfig),
                    // agentparams that represent the fixed state
                ),
            ),
        ))
        require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

        // Option B — fix via runtime action (e.g. chmod), then manual restart:
        // suite.Env().RemoteHost.MustExecute("sudo ...")
        // require.NoError(t, suite.Env().Agent.Client.Restart())
        // require.EventuallyWithT(t, func(ct *assert.CollectT) {
        //     assert.True(ct, suite.Env().Agent.Client.IsReady())
        // }, 2*time.Minute, 10*time.Second, "agent not ready after fix")
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

**If the issue ID includes a runtime-generated suffix** (e.g. an issue ID with a hash appended per instance):
add a `findIssuesByPrefix` helper to `helpers_test.go` (mirroring `findIssuesByID` but using `strings.HasPrefix`) and use it in place of `findIssuesByID(t, p, issueID)`.

---

## Step 4 — Fixture rules

| Content | How to include |
|---|---|
| Short YAML (≤ 10 lines) | `const` string literal inline in the test file |
| Python file | `//go:embed fixtures/{module}.py` → `var {module}Py string` |
| Long YAML | `//go:embed fixtures/{module}_config.yaml` → `var {module}Config string` |

Before adding a new agent config fixture, check whether an existing one in the package already fits — reuse it instead of declaring a near-duplicate.

---

## Step 5 — Rules (never break these)

- **Flush order**: restart or `UpdateEnv` first → wait for agent ready → `FlushServerAndResetAggregators()`. Never flush before restarting.
- **No resilience phase**: `RestartResilience` is covered once in `TestResilienceSuite`. Do not add it here.
- **No diagnose assertions**: assert only through fakeintake state (`ISSUE_STATE_ACTIVE`, `ISSUE_STATE_RESOLVED`).
- **No shared lifecycle driver**: phases (`IssueDetection`, `Resolution`) are always inline in the test method.
- **No agent-ready wait at suite start**: the framework guarantees the agent is ready before the first test method runs.
- **No `Diagnose()` on the env struct**.
- **Always call `t.Parallel()`**: the first line of every `Test{Module}Suite` function must be `t.Parallel()` so the suite can run concurrently with others.

---

## Step 6 — Verify and report

```bash
dda inv linter.go --targets=test/new-e2e/tests/agent-health
dda inv new-e2e-tests.run --targets=./tests/agent-health/... --run=^Test{Module}Suite$
```

Tell the user:
- The file that was created
- How to run the test locally: `dda inv new-e2e-tests.run --targets=./tests/agent-health/... --run=^Test{Module}Suite$`
- Any fixtures that need to be created in `fixtures/`
