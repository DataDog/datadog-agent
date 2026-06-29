# ECS E2E Tests

## Overview

This directory contains end-to-end tests for the Datadog Agent on Amazon Elastic Container Service (ECS). These tests validate agent functionality across all three ECS deployment scenarios: **Fargate**, **EC2**, and **Managed Instances**.

### Ownership

**Team**: ecs-experiences
**Purpose**: Validate Datadog Agent behavior in ECS environments
**Coverage**: All telemetry types (metrics, logs, traces) and all ECS deployment types

---

## Test Organization

All ECS tests run under a single test suite (`TestECSSuite`) that provisions
**one** ECS cluster with all required capacity providers (Fargate, Linux EC2,
Linux BottleRocket, Managed Instances). Cluster spawn time dominates test
runtime, so running everything on a shared cluster avoids both the cost and
the AWS quota pressure of spawning a fresh cluster per test class.

Tests are grouped into thematic files for readability — they are all methods
on the same `ecsSuite` type:

### `apm_test.go` — APM/tracing & DogStatsD
- `Test00UpAndRunning` — Infrastructure readiness (runs first via `00` prefix)
- `Test01AgentAPMReady` — APM agent readiness, validated by trace arrival
- `TestDogstatsdUDS` / `TestDogstatsdUDP` — DogStatsD over UDS / UDP
- `TestTraceUDS` / `TestTraceTCP` — Trace collection over UDS / TCP, validating
  bundled `_dd.tags.container` ECS metadata

### `checks_test.go` — Check autodiscovery & execution
- `TestNginxECS` / `TestRedisECS` — Docker-label and image-name autodiscovery on EC2
- `TestNginxFargate` / `TestRedisFargate` — Same checks on Fargate
- `TestPrometheus` — Prometheus/OpenMetrics check

### `platform_test.go` — Platform-specific features
- `TestCPU` — CPU metrics with value-range validation (stress-ng workload)
- `TestContainerLifecycle` — Multi-container lifecycle metric tracking

### `managed_test.go` — Managed Instance specifics
- `TestManagedInstanceAgentHealth` — Agent health on managed instances
- `TestManagedInstanceTraceCollection` — Trace collection with ECS metadata

---

## Running Tests

### Prerequisites

- **AWS credentials**: Configure AWS CLI with appropriate permissions
- **Pulumi**: Infrastructure provisioning (installed by `dda inv install-tools`)
- **Go**: Version specified in `go.mod`
- **Datadog API key**: Set in environment (handled by test framework)

### Running the suite

```bash
# Run all ECS tests on a single shared cluster
dda inv new-e2e-tests.run --targets=./tests/ecs/...
```

To run a single test method, use `-test.run`:

```bash
dda inv new-e2e-tests.run --targets=./tests/ecs/... \
  --extra-flags="-test.run TestECSSuite/TestTraceUDS"
```

---

## Coverage Matrix

| Feature                | Fargate | EC2 | Managed | Tests                      |
|------------------------|---------|-----|---------|----------------------------|
| Metrics collection     | Yes     | Yes | Yes     | checks_test, platform_test |
| Log collection         | Yes     | Yes | -       | checks_test                |
| APM traces             | -       | Yes | Yes     | apm_test, managed_test     |
| Check autodiscovery    | Yes     | Yes | -       | checks_test                |
| DogStatsD              | -       | Yes | -       | apm_test                   |
| Container lifecycle    | Yes     | Yes | -       | platform_test              |
| Prometheus             | -       | Yes | -       | checks_test                |
| BottleRocket           | -       | Yes | -       | platform_test              |

---

## Related Documentation

- [E2E Framework Guide](../../../e2e-framework/README.md)
- [FakeIntake Documentation](../../../fakeintake/README.md)
- [ECS Fargate Integration](https://docs.datadoghq.com/integrations/ecs_fargate/)
- [ECS EC2 Integration](https://docs.datadoghq.com/agent/amazon_ecs/)
