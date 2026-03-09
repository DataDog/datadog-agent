---
name: write-e2e
description: Write E2E tests for the Datadog Agent using the new-e2e framework with fakeintake assertions
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Agent
argument-hint: "<feature-or-check-name> [--platform linux|windows|both] [--env host|docker|k8s]"
---

Write end-to-end tests for the Datadog Agent using the `test/new-e2e/` framework.

## Instructions

### 1. Understand the feature under test

Parse `$ARGUMENTS` to determine what to test. The user may provide:
- A check name (e.g., `ntp`, `cpu`, `network`)
- A feature area (e.g., `log collection`, `flare`, `health`)
- A specific behavior to validate

**Research the feature** before writing any code:
- Read the implementation in `pkg/` or `comp/` to understand what metrics, logs, service checks, or behaviors it produces
- Read existing unit tests to understand edge cases
- Check if E2E tests already exist under `test/new-e2e/tests/`
- Identify the exact metric names, tags, service check names, and expected values

### 2. Choose the test environment

Based on what you're testing, pick the right environment type:

| Environment | When to use | Import |
|-------------|-------------|--------|
| `environments.Host` | System checks, agent commands, file-based config | `awshost.Provisioner()` |
| `environments.DockerHost` | Container checks, Docker integrations | Docker provisioner |
| `environments.Kubernetes` | K8s checks, cluster agent, DaemonSet | K8s provisioner |

Default to `environments.Host` with `awshost.Provisioner()` for most checks.

### 3. Write the test files

Place tests under `test/new-e2e/tests/<area>/` following the project conventions:

**File organization:**
```
test/new-e2e/tests/<area>/
├── <name>_common_test.go     # Base suite, shared helpers, assertion logic
├── <name>_nix_test.go        # Linux entry point (if Linux-specific)
├── <name>_win_test.go        # Windows entry point (if Windows-specific)
└── fixtures/                 # Embedded config files (YAML, Python checks)
    └── <config>.yaml
```

If the test is platform-independent, a single `<name>_test.go` file is fine.

**Required structure for each test file:**

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package <name> contains e2e tests for <feature>
package <name>

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"

    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
    awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)
```

### 4. Define the test suite

```go
type <name>Suite struct {
    e2e.BaseSuite[environments.Host]
}

func TestXxx(t *testing.T) {
    t.Parallel()
    e2e.Run(t, &<name>Suite{}, e2e.WithProvisioner(
        awshost.Provisioner(
            // Add options as needed
        ),
    ))
}
```

### 5. Configure the agent

Use `agentparams` to configure the agent for your test:

```go
import (
    "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
    ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

// Enable a check with custom config
awshost.Provisioner(
    awshost.WithRunOptions(
        ec2.WithAgentOptions(
            agentparams.WithAgentConfig(agentConfig),
            agentparams.WithIntegration("check_name.d", checkConfig),
        ),
    ),
)
```

Common `agentparams` options:
- `WithAgentConfig(yaml)` — main datadog.yaml overrides
- `WithIntegration(name, yaml)` — add a check config under conf.d/
- `WithLogs()` — enable log collection
- `WithSystemProbeConfig(yaml)` — system-probe config
- `WithFile(path, content, executable)` — place a file on the host

### 6. Write assertions with fakeintake

The fakeintake captures everything the agent sends. Use it to validate metrics, logs, service checks, and more.

**Metrics:**
```go
import (
    "github.com/DataDog/datadog-agent/test/fakeintake/client"
    "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func (s *mySuite) TestMetricsArriveInFakeintake() {
    s.EventuallyWithT(func(c *assert.CollectT) {
        metrics, err := s.Env().FakeIntake.Client().FilterMetrics("metric.name",
            client.WithMetricValueHigherThan(0),
        )
        assert.NoError(c, err)
        assert.NotEmpty(c, metrics, "no 'metric.name' metrics yet")
    }, 5*time.Minute, 10*time.Second)
}
```

**Service checks:**
```go
func (s *mySuite) TestServiceCheckReported() {
    s.EventuallyWithT(func(c *assert.CollectT) {
        checkRuns, err := s.Env().FakeIntake.Client().FilterCheckRuns("check.name")
        assert.NoError(c, err)
        assert.NotEmpty(c, checkRuns, "no 'check.name' service check yet")
    }, 5*time.Minute, 10*time.Second)
}
```

**Logs:**
```go
import fi "github.com/DataDog/datadog-agent/test/fakeintake/client"

func (s *mySuite) TestLogsArriveInFakeintake() {
    // Generate a log
    s.Env().RemoteHost.MustExecute("echo 'test message' >> /tmp/test.log")

    s.EventuallyWithT(func(c *assert.CollectT) {
        logs, err := s.Env().FakeIntake.Client().FilterLogs("service_name",
            fi.WithMessageContaining("test message"),
        )
        assert.NoError(c, err)
        assert.NotEmpty(c, logs)
    }, 5*time.Minute, 10*time.Second)
}
```

**Tag filtering:**
```go
import "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

// Filter metrics by tags
metrics, err := s.Env().FakeIntake.Client().FilterMetrics("metric.name",
    client.WithTags[*aggregator.MetricSeries]([]string{"key:value"}),
)
```

### 7. Timeouts and intervals

- **Default assertion timeout**: 5 minutes (infrastructure may take time to report)
- **Default poll interval**: 10 seconds
- **NTP/infrequent checks**: The check may have a long interval (e.g., NTP = 15 min). Adjust the timeout accordingly or override the check's `min_collection_interval` in the config.
- Use `EventuallyWithT` (not `Eventually`) — it provides `*assert.CollectT` for proper assertion collection.

### 8. Platform-specific tests

For tests that need to run on specific platforms:

**Linux entry point (`_nix_test.go`):**
```go
func TestLinuxXxx(t *testing.T) {
    t.Parallel()
    e2e.Run(t, &linuxSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}
```

**Windows entry point (`_win_test.go`):**
```go
import e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

func TestWindowsXxx(t *testing.T) {
    t.Parallel()
    e2e.Run(t, &windowsSuite{}, e2e.WithProvisioner(
        awshost.Provisioner(
            awshost.WithRunOptions(
                ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsDefault)),
            ),
        ),
    ))
}
```

### 9. Verify the test compiles

After writing, verify the test compiles:
```bash
cd test/new-e2e && go vet ./tests/<area>/...
```

Do NOT run the test (it provisions real cloud infrastructure). Just verify compilation.

### 10. Running locally

E2E tests provision real cloud infrastructure on AWS (`agent-sandbox` account). To run
locally, Datadog agent developers can use:

```bash
dda inv new-e2e-tests.run --targets=./tests/<area>/...
```

The invoke task handles AWS authentication internally (via `aws-vault`) — you do
**not** need to wrap the command with `aws-vault exec` yourself.

Or use the `/run-e2e` skill which handles this automatically.

**Prerequisites** (see `docs/public/how-to/test/e2e.md` for full setup):
- `pulumi` CLI installed and `PULUMI_CONFIG_PASSPHRASE` set
- `~/.test_infra_config.yaml` configured (run `dda inv e2e.setup`)
- `aws-vault` installed with access to the `agent-sandbox` AWS account (Datadog agent org members)

### 11. CI integration (inform the user)

Remind the user that E2E tests need a CI job definition to run automatically. Point them to:
- `.gitlab-ci.yml` for rule definitions
- `.gitlab/e2e/e2e.yml` for job definitions
- The `run-e2e` skill (`/run-e2e`) for local execution

## Key patterns from the codebase

### Naming conventions
- Test functions: `TestXxxSuite` for the entry point, `TestXxx` for individual tests
- Suites: `xxxSuite` struct embedding `e2e.BaseSuite[environments.Host]`
- Packages: match the directory name under `test/new-e2e/tests/`

### Test ordering
Tests within a suite run alphabetically. Use prefixes to control order:
- `Test00_Setup` — runs first (warmup/validation)
- `TestZZ_Cleanup` — runs last (invariant checks)

### Environment updates mid-suite
Use `s.UpdateEnv(provisioner)` to change agent config between tests.

### Debugging helpers
- `s.Env().FakeIntake.Client().GetMetricNames()` — list all received metrics
- `s.Env().FakeIntake.Client().GetCheckRunNames()` — list all service checks
- `s.Env().FakeIntake.Client().GetLogServiceNames()` — list all log sources
- `s.Env().Agent.Client.Hostname()` — get the agent hostname

### References
- E2E framework: `test/e2e-framework/testing/e2e/suite.go`
- Environments: `test/e2e-framework/testing/environments/`
- Fakeintake client: `test/fakeintake/client/client.go`
- Example tests: `test/new-e2e/examples/`
- Real tests: `test/new-e2e/tests/` (use as reference for patterns)
- Confluence docs: https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3492282413/E2E+Framework

## Usage

- `/write-e2e ntp` — Write E2E test for the NTP check
- `/write-e2e log collection` — Write E2E test for log collection
- `/write-e2e ntp --platform both` — Write tests for both Linux and Windows
- `/write-e2e flare` — Write E2E test for the flare command

## Output

Show the user:
1. The files created/modified
2. How to compile-check the test
3. How to run it locally with `/run-e2e`
4. Whether CI integration is needed
