# ECS Multi-Service Test Application

## Overview

The ECS Multi-Service test application is a **test infrastructure component** owned by the **ecs-experiences team** for validating distributed tracing functionality in ECS environments.

## Purpose

This application exists to test and validate:

1. **Distributed Tracing**: Multi-service trace propagation across container boundaries
2. **Service Discovery**: Automatic service-to-service communication in ECS
3. **Trace Correlation**: Proper trace context propagation between services
4. **Log-Trace Correlation**: Integration of trace IDs in application logs
5. **ECS Metadata Enrichment**: Proper tagging of traces with ECS task/container metadata
6. **Platform Coverage**: Both ECS EC2 and ECS Fargate deployment scenarios

## Architecture

The application consists of a 3-tier microservices architecture:

```
┌──────────┐      ┌──────────┐      ┌──────────┐
│ Frontend │─────▶│ Backend  │─────▶│ Database │
│ (port    │ HTTP │ (port    │ HTTP │ (port    │
│  8080)   │      │  8080)   │      │  8080)   │
└──────────┘      └──────────┘      └──────────┘
     │                  │                  │
     └──────────────────┴──────────────────┘
                        │
                 Datadog Tracing
             (traces with span links)
```

### Services

1. **Frontend Service** (`frontend`)
   - Entry point for requests
   - Calls backend service
   - Emits parent spans
   - Service: `frontend`, Env: `test`, Version: `1.0`

2. **Backend Service** (`backend`)
   - API processing layer
   - Calls database service
   - Emits child spans linked to frontend
   - Service: `backend`, Env: `test`, Version: `1.0`

3. **Database Service** (`database`)
   - Simulated data layer
   - Emits leaf spans
   - Service: `database`, Env: `test`, Version: `1.0`

## Deployment Modes

### ECS EC2 (`ecs.go`)

- **Network Mode**: Bridge
- **Agent Communication**: Unix Domain Socket (UDS) via `/var/run/datadog/apm.socket`
- **Service Discovery**: Docker links (`backend:backend`, `database:database`)
- **Agent Deployment**: Daemon mode (one agent per EC2 instance)
- **Resource Allocation**: 100 CPU, 128MB memory per service

### ECS Fargate (`ecsFargate.go`)

- **Network Mode**: awsvpc
- **Agent Communication**: TCP via `http://localhost:8126`
- **Service Discovery**: Localhost communication (all containers share network namespace)
- **Agent Deployment**: Sidecar mode (agent in same task)
- **Resource Allocation**: 256 CPU, 256MB memory per service
- **Total Task Resources**: 2048 CPU, 4096MB memory

## Configuration

All services are configured with:

```bash
DD_SERVICE=<service-name>     # Service name for APM
DD_ENV=test                   # Environment tag
DD_VERSION=1.0                # Version tag
DD_LOGS_INJECTION=true        # Enable trace ID injection in logs
DD_TRACE_AGENT_URL=<url>      # Agent endpoint (UDS for EC2, TCP for Fargate)
```

### Docker Labels (EC2 only)

```
com.datadoghq.ad.tags: ["ecs_launch_type:ec2","tier:<frontend|backend|database>"]
com.datadoghq.ad.logs: [{"source":"<service>","service":"<service>"}]
```

## Docker Images

The application requires the following Docker images to be built and published:

- `ghcr.io/datadog/apps-ecs-multiservice-frontend:<version>`
- `ghcr.io/datadog/apps-ecs-multiservice-backend:<version>`
- `ghcr.io/datadog/apps-ecs-multiservice-database:<version>`

### Image Requirements

Each image should:
- Implement a simple HTTP server
- Use Datadog tracer library (ddtrace-py, dd-trace-go, or similar)
- Accept environment variables for configuration
- Make HTTP calls to downstream services based on environment variables
- Produce JSON-formatted logs with trace correlation
- Include health check endpoint

### Example Implementation (Python/Flask)

```python
from flask import Flask
from ddtrace import tracer, patch_all
import requests
import logging

patch_all()
app = Flask(__name__)

@app.route('/')
def index():
    # Make downstream call if configured
    backend_url = os.getenv('BACKEND_URL')
    if backend_url:
        requests.get(backend_url)
    return 'OK'

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8080)
```

## Usage in Tests

Import and use in E2E tests:

```go
import (
    ecsmultiservice "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/ecs-multiservice"
)

// For EC2
workload, err := ecsmultiservice.EcsAppDefinition(env, clusterArn)

// For Fargate
workload, err := ecsmultiservice.FargateAppDefinition(env, clusterArn, apiKeySSM, fakeIntake)
```

Then validate in tests:

```go
// Validate distributed tracing
traces, _ := fakeintake.GetTraces()
// Assert: traces contain frontend → backend → database spans
// Assert: spans have proper parent-child relationships
// Assert: all spans have ECS metadata tags

// Validate log-trace correlation
logs, _ := fakeintake.GetLogs()
// Assert: logs contain dd.trace_id tags
// Assert: trace IDs match between logs and traces
```

## Test Coverage

This application is used by:

- `test/new-e2e/tests/containers/ecs_apm_test.go`
  - TestMultiServiceTracing
  - TestTraceCorrelation
  - TestAPMFargate
  - TestAPMEC2

## Maintenance

**Owned by**: ecs-experiences Team
**Purpose**: Test Infrastructure
**Used for**: ECS E2E Testing

### When to Update

- When adding new distributed tracing features to test
- When ECS metadata collection changes
- When testing new APM agent features in ECS context
- When validating ECS-specific trace enrichment

### Do NOT Use For

- Production workloads
- APM product testing (use dedicated APM test apps)
- Performance benchmarking
- Load testing

## Related Documentation

- [ECS E2E Testing Plan](../../../../../../../../CLAUDE.md)
- [E2E Testing Framework](../../../../README.md)
- [ECS Test Infrastructure](../../../../../../../test-infra-definition/)

## FAQ

**Q: Why is this owned by ecs-experiences team and not APM team?**
A: This is infrastructure for testing how the **agent** collects traces in **ECS environments**. It's about validating agent functionality, not APM product features.

**Q: Can I use this for testing APM features?**
A: No. This is specifically for testing agent behavior in ECS. Use APM-owned test applications for APM feature testing.

**Q: Why not use the existing tracegen app?**
A: `tracegen` emits simple traces but doesn't test multi-service distributed tracing, which requires service-to-service communication and trace context propagation.

**Q: What about other platforms (Kubernetes, Docker)?**
A: This app is ECS-specific. Similar apps exist or should be created for other platforms (e.g., `k8s-multiservice` for Kubernetes).
