# ECS E2E Tests

## Overview

This directory contains end-to-end tests for the Datadog Agent on Amazon Elastic Container Service (ECS). These tests validate agent functionality across all three ECS deployment scenarios: **Fargate**, **EC2**, and **Managed Instances**.

### Ownership

**Team**: ecs-experiences
**Purpose**: Validate Datadog Agent behavior in ECS environments
**Coverage**: All telemetry types (metrics, logs, traces) and all ECS deployment types

---

## Test Suites

This directory contains **4 test suites** with **18 total tests**:

### 1. `apm_test.go` - APM/Tracing (6 tests)
Tests APM trace collection and DogStatsD across ECS environments.

**Tests**:
- `Test00UpAndRunning` - Infrastructure readiness check
- `Test01AgentAPMReady` - APM agent readiness check
- `TestDogstatsdUDS` - DogStatsD via Unix Domain Socket (full 23-tag regex validation)
- `TestDogstatsdUDP` - DogStatsD via UDP (full 23-tag regex validation)
- `TestTraceUDS` - Trace collection via UDS (13-pattern bundled tag validation)
- `TestTraceTCP` - Trace collection via TCP (13-pattern bundled tag validation)

**Key Features Tested**:
- ECS metadata tags (`ecs_cluster_name`, `task_arn`, `task_family`, `task_version`, etc.)
- Image metadata tags (`docker_image`, `image_name`, `image_tag`, `short_image`)
- Git metadata tags (`git.commit.sha`, `git.repository_url`)
- DogStatsD over UDS and UDP transports
- Trace collection over UDS and TCP transports

---

### 2. `checks_test.go` - Check Autodiscovery & Execution (5 tests)
Tests integration check autodiscovery and execution across deployment types.

**Tests**:
- `TestNginxECS` - Nginx check via docker labels (EC2) with full metric + log tag validation
- `TestRedisECS` - Redis check via image name autodiscovery (EC2) with full metric + log tag validation
- `TestNginxFargate` - Nginx check on Fargate with full metric tag validation
- `TestRedisFargate` - Redis check on Fargate with full metric tag validation
- `TestPrometheus` - Prometheus/OpenMetrics check with full metric tag validation

**Key Features Tested**:
- Docker label-based check configuration (`com.datadoghq.ad.check_names`)
- Image name-based autodiscovery (redis, nginx)
- Check execution on both EC2 and Fargate
- Log collection with tag validation (nginx, redis)
- Prometheus metrics scraping

---

### 3. `platform_test.go` - Platform-Specific Features (4 tests)
Tests platform-specific functionality and performance monitoring.

**Tests**:
- `Test00UpAndRunning` - Infrastructure readiness check
- `TestWindowsFargate` - Windows container support on Fargate (check run + container metric tag validation)
- `TestCPU` - CPU metrics with value range validation (stress-ng workload)
- `TestContainerLifecycle` - Container lifecycle tracking (multi-container metric validation)

**Key Features Tested**:
- Windows container monitoring on Fargate
- BottleRocket node support
- CPU metric value range validation
- Multi-container lifecycle tracking

---

### 4. `managed_test.go` - Managed Instances (3 tests)
Tests managed instance-specific features.

**Tests**:
- `Test00UpAndRunning` - Infrastructure readiness check
- `TestManagedInstanceAgentHealth` - Agent health check via AssertAgentHealth helper
- `TestManagedInstanceTraceCollection` - Trace collection with bundled tag validation

**Key Features Tested**:
- Managed instance provisioning and lifecycle
- Daemon mode agent deployment
- Trace collection with ECS metadata validation

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
go test -v -timeout 30m ./test/new-e2e/tests/ecs/ -run TestECSAPMSuite

# Run checks tests only
go test -v -timeout 30m ./test/new-e2e/tests/ecs/ -run TestECSChecksSuite

# Run platform tests only
go test -v -timeout 30m ./test/new-e2e/tests/ecs/ -run TestECSPlatformSuite

# Run managed instance tests only
go test -v -timeout 30m ./test/new-e2e/tests/ecs/ -run TestECSManagedSuite
```

### Running All ECS Tests

```bash
go test -v -timeout 60m ./test/new-e2e/tests/ecs/...
```

---

## Coverage Matrix

### Feature Coverage by Deployment Type

| Feature | Fargate | EC2 | Managed | Tests |
|---------|---------|-----|---------|-------|
| **Metrics Collection** | Yes | Yes | Yes | checks_test, platform_test |
| **Log Collection** | Yes | Yes | - | checks_test |
| **APM Traces** | - | Yes | Yes | apm_test, managed_test |
| **Check Autodiscovery** | Yes | Yes | - | checks_test |
| **DogStatsD** | - | Yes | - | apm_test |
| **Container Lifecycle** | Yes | Yes | - | platform_test |
| **Windows Support** | Yes | - | - | platform_test |
| **Prometheus** | - | Yes | - | checks_test |
| **BottleRocket** | - | Yes | - | platform_test |

---

## Related Documentation

- [E2E Framework Guide](../../../e2e-framework/README.md)
- [FakeIntake Documentation](../../../fakeintake/README.md)
- [ECS Fargate Integration](https://docs.datadoghq.com/integrations/ecs_fargate/)
- [ECS EC2 Integration](https://docs.datadoghq.com/agent/amazon_ecs/)
