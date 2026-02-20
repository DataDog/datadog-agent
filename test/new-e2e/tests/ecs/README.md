# ECS E2E Tests

## Overview

This directory contains comprehensive end-to-end tests for the Datadog Agent on Amazon Elastic Container Service (ECS). These tests validate agent functionality across all three ECS deployment scenarios: **Fargate**, **EC2**, and **Managed Instances**.

### Ownership

**Team**: ecs-experiences
**Purpose**: Validate Datadog Agent behavior in ECS environments
**Coverage**: All telemetry types (metrics, logs, traces) and all ECS deployment types

### Scope

The ECS E2E test suite covers:
- **APM/Distributed Tracing**: Trace collection, sampling, tag enrichment, correlation
- **Log Collection**: Container logs, multiline handling, parsing, filtering
- **Configuration & Discovery**: Autodiscovery, environment variables, metadata endpoints
- **Resilience**: Agent restart recovery, network interruptions, resource exhaustion
- **Platform Features**: Windows support, check execution, Prometheus integration
- **Deployment Scenarios**: Fargate (sidecar), EC2 (daemon), Managed Instances

---

## Test Suites

This directory contains **7 test suites** with **61 total tests**:

### 1. `apm_test.go` - APM/Tracing (8 tests)
Tests APM trace collection and distributed tracing across ECS environments.
Uses the standard testing workload which includes `tracegen` with `DD_LOGS_INJECTION=true` for trace-log correlation testing.

**Tests**:
- `Test00AgentAPMReady` - APM agent readiness check
- `TestBasicTraceCollection` - Basic trace ingestion and metadata
- `TestMultiServiceTracing` - Multi-service distributed tracing
- `TestTraceSampling` - Trace sampling priority validation
- `TestTraceTagEnrichment` - ECS metadata tag enrichment on traces
- `TestTraceCorrelation` - Trace-log correlation (trace_id in logs)
- `TestAPMFargate` - Fargate-specific APM (TCP transport, sidecar)
- `TestAPMEC2` - EC2-specific APM (UDS transport, daemon mode)

**Key Features Tested**:
- Trace structure validation (TraceID, SpanID, ParentID)
- Sampling priority (`_sampling_priority_v1` metric)
- ECS metadata tags (`ecs_cluster_name`, `task_arn`, etc.)
- Parent-child span relationships
- Launch type detection (fargate vs ec2)

---

### 2. `logs_test.go` - Log Collection (9 tests)
Tests log collection, processing, and enrichment from ECS containers.
Uses the standard testing workload which includes `tracegen` with `DD_LOGS_INJECTION=true` for trace-log correlation testing.

**Tests**:
- `Test00AgentLogsReady` - Log agent readiness check
- `TestContainerLogCollection` - Basic container log collection with metadata
- `TestLogMultiline` - Multiline log handling (stack traces)
- `TestLogParsing` - JSON log parsing and structured log extraction
- `TestLogSampling` - High-volume log sampling
- `TestLogFiltering` - Include/exclude pattern filtering
- `TestLogSourceDetection` - Automatic source field detection
- `TestLogStatusRemapping` - Error/warning status detection
- `TestLogTraceCorrelation` - Trace ID injection into logs

**Key Features Tested**:
- Log metadata enrichment (cluster, task, container tags)
- Multiline patterns (stack trace grouping)
- JSON parsing and field extraction
- Log status detection (error, warning, info)
- Trace correlation (`dd.trace_id` tag)

---

### 3. `config_test.go` - Configuration & Discovery (7 tests)
Tests agent configuration, autodiscovery, and metadata collection.

**Tests**:
- `TestEnvVarConfiguration` - `DD_*` environment variable propagation
- `TestDockerLabelDiscovery` - `com.datadoghq.ad.*` label-based config
- `TestTaskDefinitionDiscovery` - Task definition metadata usage
- `TestDynamicConfiguration` - Container discovery and dynamic config updates
- `TestMetadataEndpoints` - ECS metadata endpoint usage (V1/V2/V3/V4)
- `TestServiceDiscovery` - Service name detection and tagging
- `TestConfigPrecedence` - Configuration priority (env vars vs labels vs defaults)

**Key Features Tested**:
- `DD_TAGS`, `DD_SERVICE`, `DD_ENV`, `DD_VERSION` propagation
- Docker label autodiscovery (`com.datadoghq.ad.check_names`, etc.)
- Task/container metadata endpoint access
- Dynamic container discovery
- Configuration precedence rules

---

### 4. `resilience_test.go` - Resilience & Error Handling (8 tests)
Tests agent behavior under failure and stress conditions.

**Tests**:
- `TestAgentRestart` - Agent restart recovery and data collection resumption
- `TestTaskFailureRecovery` - Task replacement monitoring
- `TestNetworkInterruption` - Network outage handling and data buffering
- `TestHighCardinality` - High cardinality metric handling
- `TestResourceExhaustion` - Low memory/CPU behavior
- `TestRapidContainerChurn` - Fast container lifecycle tracking
- `TestLargePayloads` - Large trace/log payload handling
- `TestBackpressure` - Slow downstream (fakeintake) handling

**Key Features Tested**:
- Data collection continuity after agent restart
- Task failure detection and replacement tracking
- Network interruption buffering
- Cardinality explosion handling
- Memory/CPU pressure graceful degradation
- Container churn without memory leaks

---

### 5. `managed_test.go` - Managed Instances (12 tests)
Tests managed instance-specific features and deployment scenarios.

**Tests**:
- `TestManagedInstanceBasicMetrics` - Basic metric collection
- `TestManagedInstanceMetadata` - ECS metadata enrichment
- `TestManagedInstanceAgentHealth` - Agent health checks
- `TestManagedInstanceContainerDiscovery` - Container discovery
- `TestManagedInstanceTaskTracking` - Task tracking
- `TestManagedInstanceDaemonMode` - Daemon mode validation
- `TestManagedInstanceLogCollection` - Log collection
- `TestManagedInstanceTraceCollection` - Trace collection
- `TestManagedInstanceNetworkMode` - Bridge networking
- `TestManagedInstanceAutoscalingIntegration` - Autoscaling behavior
- `TestManagedInstancePlacementStrategy` - Task placement
- `TestManagedInstanceResourceUtilization` - Resource metrics

**Key Features Tested**:
- Managed instance provisioning and lifecycle
- ECS-managed autoscaling integration
- Instance draining behavior
- Daemon mode agent deployment
- Placement strategy validation

---

### 6. `checks_test.go` - Check Autodiscovery & Execution (5 tests)
Tests integration check autodiscovery and execution across deployment types.

**Tests**:
- `TestNginxECS` - Nginx check via docker labels (EC2)
- `TestRedisECS` - Redis check via image name autodiscovery (EC2)
- `TestNginxFargate` - Nginx check on Fargate
- `TestRedisFargate` - Redis check on Fargate
- `TestPrometheus` - Prometheus/OpenMetrics check

**Key Features Tested**:
- Docker label-based check configuration (`com.datadoghq.ad.check_names`)
- Image name-based autodiscovery (redis, nginx)
- Check execution on both EC2 and Fargate
- Check metric collection with proper ECS tags
- Prometheus metrics scraping

---

### 7. `platform_test.go` - Platform-Specific Features (3 tests)
Tests platform-specific functionality and performance monitoring.

**Tests**:
- `TestWindowsFargate` - Windows container support on Fargate
- `TestCPU` - CPU metrics with value validation (stress test)
- `TestContainerLifecycle` - Container lifecycle tracking

**Key Features Tested**:
- Windows container monitoring on Fargate
- Windows-specific tags and metrics
- CPU metric value range validation
- Stress workload monitoring
- Multi-container lifecycle tracking

---

## Architecture

### Test Infrastructure

```
┌─────────────────────────────────────────────────────────────┐
│                     E2E Test Framework                       │
│                                                               │
│  ┌───────────────┐      ┌──────────────┐                    │
│  │   Pulumi      │─────▶│  AWS ECS     │                    │
│  │ Provisioner   │      │  Resources   │                    │
│  └───────────────┘      └──────────────┘                    │
│                                │                              │
│                                ▼                              │
│  ┌───────────────────────────────────────────┐              │
│  │          ECS Cluster                       │              │
│  │  ┌─────────────┐  ┌──────────────┐       │              │
│  │  │ Fargate     │  │ EC2 Instances│       │              │
│  │  │ Tasks       │  │ + Daemon     │       │              │
│  │  └─────────────┘  └──────────────┘       │              │
│  │         │                 │                │              │
│  │         ▼                 ▼                │              │
│  │  ┌──────────────────────────────┐        │              │
│  │  │  Datadog Agent Containers     │        │              │
│  │  │  (sidecar or daemon mode)     │        │              │
│  │  └──────────────────────────────┘        │              │
│  └───────────────────────────────────────────┘              │
│                                │                              │
│                                ▼                              │
│  ┌───────────────────────────────────────────┐              │
│  │          FakeIntake                        │              │
│  │  (validates metrics, logs, traces)         │              │
│  └───────────────────────────────────────────┘              │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Test Applications

All suites use the shared testing workload via `scenecs.WithTestingWorkload()`, which deploys standard test applications (redis, nginx, tracegen, dogstatsd, cpustress, prometheus) on EC2 and Fargate launch types. The `tracegen` app has `DD_LOGS_INJECTION=true` enabled for trace-log correlation testing.

The managed instance suite additionally deploys `tracegen` explicitly via `scenecs.WithWorkloadApp()` since `WithTestingWorkload()` only deploys on EC2 capacity providers.

### Deployment Scenarios

| Scenario | Network Mode | Agent Mode | Trace Transport | Use Case |
|----------|--------------|------------|-----------------|----------|
| **Fargate** | awsvpc | Sidecar | TCP (localhost:8126) | Serverless workloads |
| **EC2** | bridge | Daemon | UDS (/var/run/datadog/apm.socket) | Full control, daemon mode |
| **Managed** | bridge | Daemon | UDS | AWS-managed scaling |

---

## Running Tests

### Prerequisites

- **AWS credentials**: Configure AWS CLI with appropriate permissions
- **Pulumi**: Infrastructure provisioning (installed by `dda inv install-tools`)
- **Go**: Version specified in `go.mod`
- **Datadog API key**: Set in environment (handled by test framework)

### Running Individual Suites

```bash
# Run APM tests only
go test -v -timeout 30m ./test/new-e2e/tests/ecs/apm_test.go

# Run logs tests only
go test -v -timeout 30m ./test/new-e2e/tests/ecs/logs_test.go

# Run resilience tests (longer timeout)
go test -v -timeout 60m ./test/new-e2e/tests/ecs/resilience_test.go
```

### Running All ECS Tests

```bash
# Run all ECS tests in parallel
go test -v -timeout 60m ./test/new-e2e/tests/ecs/...

# Run with specific parallelism
go test -v -timeout 60m -parallel 3 ./test/new-e2e/tests/ecs/...
```

### Running Specific Tests

```bash
# Run single test method
go test -v -timeout 30m ./test/new-e2e/tests/ecs/apm_test.go -run TestBasicTraceCollection

# Run tests matching pattern
go test -v -timeout 30m ./test/new-e2e/tests/ecs/... -run ".*Fargate"
```

### CI/CD Integration

```bash
# Smoke tests (< 10 min) - Run on every PR
go test -tags smoke -timeout 15m ./test/new-e2e/tests/ecs/{apm,logs,config}_test.go

# Integration tests (< 30 min) - Run on merge to main
go test -timeout 45m ./test/new-e2e/tests/ecs/...

# Stress tests (< 60 min) - Run on-demand or nightly
go test -tags stress -timeout 90m ./test/new-e2e/tests/ecs/resilience_test.go
```

### Environment Variables

```bash
# Override default timeouts
export E2E_TIMEOUT_SCALE=2.0  # Double all timeouts

# Enable verbose logging
export E2E_VERBOSE=1

# Skip infrastructure teardown (for debugging)
export E2E_SKIP_TEARDOWN=1
```

---

## Test Patterns

### Suite Structure

All ECS test suites follow this structure:

```go
package ecs

import (
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type ecsAPMSuite struct {
    BaseSuite[environments.ECS]
    ecsClusterName string
}

func TestECSAPMSuite(t *testing.T) {
    t.Parallel()  // Enable parallel execution
    e2e.Run(t, &ecsAPMSuite{}, e2e.WithProvisioner(provecs.Provisioner(
        provecs.WithRunOptions(
            scenecs.WithECSOptions(
                scenecs.WithFargateCapacityProvider(),
                scenecs.WithLinuxNodeGroup(),
            ),
            scenecs.WithTestingWorkload(),
        ),
    )))
}

func (suite *ecsAPMSuite) SetupSuite() {
    suite.BaseSuite.SetupSuite()
    suite.Fakeintake = suite.Env().FakeIntake.Client()
    suite.ecsClusterName = suite.Env().ECSCluster.ClusterName
    suite.ClusterName = suite.Env().ECSCluster.ClusterName
}
```

### Helper Methods from BaseSuite

The `BaseSuite` (defined in `base.go`) provides helper methods for common validations:

```go
// Metric validation
suite.AssertMetric(&TestMetricArgs{
    Filter: TestMetricFilterArgs{
        Name: "nginx.net.request_per_s",
        Tags: []string{"^ecs_launch_type:ec2$"},
    },
    Expect: TestMetricExpectArgs{
        Tags: &[]string{`^cluster_name:.*`, `^task_arn:.*`},
        Value: &TestMetricExpectValueArgs{Min: 0, Max: 1000},
    },
})

// Log validation
suite.AssertLog(&TestLogArgs{
    Filter: TestLogFilterArgs{
        Service: "nginx",
        Tags: []string{"^ecs_cluster_name:.*"},
    },
    Expect: TestLogExpectArgs{
        Tags: &[]string{`^container_name:.*`},
        Message: `GET / HTTP/1\.1`,
    },
})

// APM trace validation
suite.AssertAPMTrace(&TestAPMTraceArgs{
    Filter: TestAPMTraceFilterArgs{
        ServiceName: "frontend",
    },
    Expect: TestAPMTraceExpectArgs{
        SpanCount: pointer.Int(3),
        Tags: &[]string{`^trace_id:[[:xdigit:]]+$`},
    },
})

// Agent health check
suite.AssertAgentHealth(&TestAgentHealthArgs{
    CheckComponents: []string{"logs", "trace"},
})
```

### EventuallyWithT Pattern

All assertions use `EventuallyWithTf` to handle eventual consistency:

```go
suite.EventuallyWithTf(func(c *assert.CollectT) {
    metrics, err := getAllMetrics(suite.Fakeintake)
    if !assert.NoErrorf(c, err, "Failed to query metrics") {
        return
    }
    assert.NotEmptyf(c, metrics, "No metrics found")

    // ... additional assertions
}, 2*time.Minute, 10*time.Second, "Test description")
```

**Pattern Notes**:
- **Timeout**: Typically 2-5 minutes (use `suite.Minute` for clarity)
- **Interval**: Usually 10 seconds between retries
- **Fail Fast**: Return early on assertion failures to avoid cascading errors

### FakeIntake Validation

```go
// Get all metrics (using helper function)
metrics, err := getAllMetrics(suite.Fakeintake)

// Filter metrics by name
cpuMetrics, err := suite.Fakeintake.FilterMetrics("container.cpu.usage")

// Get all logs (using helper function)
logs, err := getAllLogs(suite.Fakeintake)

// Filter logs by service
appLogs, err := suite.Fakeintake.FilterLogs("my-service")

// Get traces
traces, err := suite.Fakeintake.GetTraces()

// Flush data (useful for testing data collection after events)
suite.Fakeintake.FlushServerAndResetAggregators()
```

---

## Adding New Tests

### Choosing the Right Suite

| Test Type | Add to Suite |
|-----------|--------------|
| APM/Tracing functionality | `apm_test.go` |
| Log collection/processing | `logs_test.go` |
| Configuration/Discovery | `config_test.go` |
| Resilience/Error handling | `resilience_test.go` |
| Check integration | `checks_test.go` |
| Platform-specific (Windows, stress) | `platform_test.go` |
| Managed instance features | `managed_test.go` |

### Test Naming Conventions

1. **Foundation tests**: `Test00*` (runs first, ensures infrastructure ready)
2. **Feature tests**: `Test<Feature><Scenario>` (e.g., `TestTraceSamplingFargate`)
3. **Integration tests**: `Test<Component1><Component2>` (e.g., `TestLogTraceCorrelation`)

### Example Test Skeleton

```go
func (suite *ecsAPMSuite) TestNewFeature() {
    suite.Run("Feature description", func() {
        suite.EventuallyWithTf(func(c *assert.CollectT) {
            // 1. Query data from FakeIntake
            traces, err := suite.Fakeintake.GetTraces()
            if !assert.NoErrorf(c, err, "Failed to query traces") {
                return
            }

            // 2. Validate data exists
            if !assert.NotEmptyf(c, traces, "No traces found") {
                return
            }

            // 3. Validate specific feature
            foundFeature := false
            for _, trace := range traces {
                if /* feature condition */ {
                    foundFeature = true
                    break
                }
            }

            // 4. Assert feature works
            assert.Truef(c, foundFeature, "Feature not working")

        }, 3*suite.Minute, 10*suite.Second, "Feature validation failed")
    })
}
```

### Required Assertions

Every test should validate:
1. **Data exists**: `assert.NotEmpty` or `assert.GreaterOrEqual`
2. **Correct tags**: Match expected ECS metadata tags
3. **Correct format**: Validate data structure (TraceID format, timestamp, etc.)
4. **Feature-specific**: Validate the actual feature being tested

---

---

## Debugging Failed Tests

### Common Failure Patterns

#### 1. **Timeout Waiting for Data**
**Symptom**: `Test timed out after 2m0s`

**Causes**:
- Agent not collecting data
- Wrong cluster/task targeted
- FakeIntake not receiving data

**Debug Steps**:
```bash
# Check agent logs
kubectl logs <agent-pod> -n datadog

# Check FakeIntake logs
kubectl logs <fakeintake-pod>

# Verify agent is running
aws ecs describe-tasks --cluster <cluster> --tasks <task-arn>
```

#### 2. **Missing Tags**
**Symptom**: `Expected tag 'ecs_cluster_name:*' not found`

**Causes**:
- Agent tagger not initialized
- Metadata endpoint unreachable
- Wrong launch type

**Debug Steps**:
- Check `Test00UpAndRunning` passes (ensures warmup)
- Verify ECS metadata endpoint accessible from container
- Check agent tagger status via agent API

#### 3. **Wrong Tag Values**
**Symptom**: `Tag 'ecs_launch_type:ec2' expected, got 'ecs_launch_type:fargate'`

**Causes**:
- Test running on wrong launch type
- Provisioner configured incorrectly

**Debug Steps**:
- Review test provisioner configuration
- Check `scenecs.WithECSOptions()` settings
- Verify correct capacity provider used

### Accessing Task Logs

```bash
# Get task ARN
aws ecs list-tasks --cluster <cluster-name>

# Get task details
aws ecs describe-tasks --cluster <cluster-name> --tasks <task-arn>

# Get CloudWatch logs (if configured)
aws logs tail /ecs/<task-family>/<container-name> --follow

# For Fargate, use ECS exec
aws ecs execute-command --cluster <cluster> --task <task-arn> \
    --container <container-name> --interactive --command "/bin/bash"
```

### FakeIntake Inspection

```go
// In test, add debug logging
metrics, _ := getAllMetrics(suite.Fakeintake)
for _, m := range metrics {
    suite.T().Logf("Metric: %s, Tags: %v", m.Metric, m.GetTags())
}

// Check FakeIntake health
resp, _ := http.Get("http://fakeintake:8080/health")
// Should return 200 OK
```

### Timing-Related Issues

If tests are flaky due to timing:
1. Increase `EventuallyWithTf` timeout
2. Add explicit `time.Sleep()` after operations
3. Flush FakeIntake and wait: `suite.Fakeintake.FlushServerAndResetAggregators(); time.Sleep(30*time.Second)`
4. Check agent flush intervals in configuration

---

## Coverage Matrix

### Feature Coverage by Deployment Type

| Feature | Fargate | EC2 | Managed | Tests |
|---------|---------|-----|---------|-------|
| **Metrics Collection** | ✅ | ✅ | ✅ | checks_test, platform_test |
| **Log Collection** | ✅ | ✅ | ✅ | logs_test |
| **APM Traces** | ✅ | ✅ | ✅ | apm_test |
| **Check Autodiscovery** | ✅ | ✅ | ✅ | checks_test |
| **ECS Metadata** | ✅ | ✅ | ✅ | config_test |
| **Container Lifecycle** | ✅ | ✅ | ✅ | platform_test, resilience_test |
| **Daemon Mode** | ❌ | ✅ | ✅ | managed_test |
| **UDS Transport** | ❌ | ✅ | ✅ | apm_test |
| **TCP Transport** | ✅ | ✅ | ✅ | apm_test |
| **Windows Support** | ✅ | ⚠️ | ⚠️ | platform_test |
| **Prometheus** | ⚠️ | ✅ | ✅ | checks_test |

Legend: ✅ Full support | ⚠️ Partial support | ❌ Not applicable

### Test Execution Time Estimates

| Suite | Tests | EC2 | Fargate | Managed | Notes |
|-------|-------|-----|---------|---------|-------|
| apm_test | 8 | ~8 min | ~10 min | ~8 min | Trace collection delays |
| logs_test | 9 | ~6 min | ~7 min | ~6 min | Log buffering |
| config_test | 7 | ~5 min | ~6 min | ~5 min | Metadata endpoint access |
| resilience_test | 8 | ~15 min | ~12 min | ~15 min | Chaos scenarios take longer |
| managed_test | 12 | N/A | N/A | ~18 min | Managed instance specific |
| checks_test | 5 | ~7 min | ~8 min | ~7 min | Check execution time |
| platform_test | 3 | ~10 min | ~12 min | ~10 min | Windows + stress tests |
| **Total** | **61** | **~51 min** | **~55 min** | **~69 min** | With parallelism: ~30 min |

---

## Related Documentation

### Agent Documentation
- [ECS Fargate Integration](https://docs.datadoghq.com/integrations/ecs_fargate/)
- [ECS EC2 Integration](https://docs.datadoghq.com/agent/amazon_ecs/)
- [ECS Autodiscovery](https://docs.datadoghq.com/agent/amazon_ecs/apm/)
- [ECS APM Setup](https://docs.datadoghq.com/tracing/setup_overview/setup/dotnet/?tab=containers)

### Test Framework Documentation
- [E2E Framework Guide](../../../e2e-framework/README.md)
- [FakeIntake Documentation](../../../fakeintake/README.md)
- [Pulumi Provisioners](../../../e2e-framework/testing/provisioners/aws/ecs/README.md)

### ECS-Specific Agent Features
- **Metadata Endpoint**: V3/V4 for Fargate, V1/V2 for EC2
- **Network Modes**: `awsvpc` (Fargate), `bridge`/`host` (EC2)
- **Agent Modes**: Sidecar (Fargate), Daemon (EC2/Managed)
- **Trace Transport**: TCP (Fargate), UDS (EC2/Managed)

### Contributing
When adding new tests to this directory:
1. Follow existing test patterns and naming conventions
2. Use helper methods from `BaseSuite` when possible
3. Add test description to this README
4. Update coverage matrix if new feature coverage added
5. Ensure tests work on all deployment types (Fargate, EC2, Managed) or document limitations

### Support
For questions or issues with these tests:
- **Slack**: #container-integrations
- **GitHub Issues**: Tag with `team/container-integrations`
- **Owners**: See CODEOWNERS file
