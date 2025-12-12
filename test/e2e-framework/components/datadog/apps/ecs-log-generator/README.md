# ECS Log Generator Test Application

## Overview

The ECS Log Generator test application is a **test infrastructure component** owned by the **ecs-experiences team** for validating log collection functionality in ECS environments.

## Purpose

This application exists to test and validate:

1. **Container Log Collection**: Stdout/stderr log collection from ECS containers
2. **Multiline Handling**: Stack traces and multiline log grouping
3. **Log Parsing**: JSON parsing, structured logs, custom parsing rules
4. **Log Filtering**: Include/exclude rules, regex patterns, log sampling
5. **Source Detection**: Automatic source detection and service attribution
6. **Status Remapping**: Error/warning level detection and custom status mapping
7. **Trace Correlation**: Log-trace correlation via trace_id injection
8. **Volume Handling**: High-volume log collection and sampling behavior

## Architecture

The application is a simple log generator that emits various log types:

```
┌─────────────────┐
│ Log Generator   │
│  - JSON logs    │
│  - Stack traces │
│  - Error logs   │
│  - High volume  │
└─────────────────┘
        │
        ▼
   Stdout/Stderr
        │
        ▼
 Datadog Agent
 (Log Collection)
        │
        ▼
   FakeIntake
```

### Configuration

The log generator supports environment variables to control behavior:

```bash
LOG_LEVEL=INFO          # Log level: DEBUG, INFO, WARN, ERROR
LOG_FORMAT=json         # Format: json, text, or mixed
LOG_RATE=10             # Logs per second (for volume testing)
EMIT_MULTILINE=true     # Emit stack traces for multiline testing
EMIT_ERRORS=true        # Emit ERROR level logs for status remapping tests

# Datadog configuration
DD_SERVICE=log-generator
DD_ENV=test
DD_VERSION=1.0
DD_LOGS_INJECTION=true  # Enable trace correlation
```

### Log Types Emitted

1. **Structured JSON Logs**
```json
{"timestamp":"2025-01-10T12:00:00Z","level":"INFO","message":"Application started","service":"log-generator"}
```

2. **Multiline Stack Traces**
```
Exception in thread "main" java.lang.NullPointerException
    at com.example.MyClass.method(MyClass.java:42)
    at com.example.Application.main(Application.java:15)
```

3. **Error Logs** (for status remapping)
```
ERROR: Database connection failed
```

4. **High-Volume Logs** (configurable rate for sampling tests)
```
INFO: Request processed [ID: 1001]
INFO: Request processed [ID: 1002]
...
```

5. **Trace-Correlated Logs**
```json
{"level":"INFO","message":"Request handled","dd.trace_id":"1234567890","dd.span_id":"9876543210"}
```

## Deployment Modes

### ECS EC2 (`ecs.go`)

- **Network Mode**: Bridge
- **Log Collection**: Docker log driver → Datadog agent (daemon mode)
- **Resource Allocation**: 100 CPU, 128MB memory
- **Docker Labels**:
  - `com.datadoghq.ad.logs`: Configure log source and service
  - `com.datadoghq.ad.log_processing_rules`: Multiline pattern for stack traces

### ECS Fargate (`ecsFargate.go`)

- **Network Mode**: awsvpc
- **Log Collection**: Firelens → Datadog agent (sidecar mode)
- **Resource Allocation**: 256 CPU, 512MB memory
- **Total Task Resources**: 1024 CPU, 2048MB memory
- **Docker Labels**: Same as EC2 for consistency

## Docker Image

The application requires the Docker image to be built and published:

- `ghcr.io/datadog/apps-ecs-log-generator:<version>`

### Image Requirements

The image should:
- Implement a log generator that emits various log types
- Support environment variable configuration
- Emit to stdout/stderr (captured by Docker/Firelens)
- Include health check endpoint (HTTP server on port 8080)
- Support configurable log rate, format, and types

### Example Implementation (Python)

```python
import json
import logging
import time
import os
from flask import Flask

app = Flask(__name__)

# Configuration
LOG_LEVEL = os.getenv('LOG_LEVEL', 'INFO')
LOG_FORMAT = os.getenv('LOG_FORMAT', 'json')
LOG_RATE = int(os.getenv('LOG_RATE', '10'))
EMIT_MULTILINE = os.getenv('EMIT_MULTILINE', 'true').lower() == 'true'
EMIT_ERRORS = os.getenv('EMIT_ERRORS', 'true').lower() == 'true'

# Setup logging
if LOG_FORMAT == 'json':
    logging.basicConfig(
        format='{"timestamp":"%(asctime)s","level":"%(levelname)s","message":"%(message)s"}',
        level=getattr(logging, LOG_LEVEL)
    )
else:
    logging.basicConfig(
        format='%(asctime)s - %(levelname)s - %(message)s',
        level=getattr(logging, LOG_LEVEL)
    )

logger = logging.getLogger(__name__)

def emit_logs():
    """Background task to emit logs at configured rate"""
    counter = 0
    while True:
        # Normal log
        logger.info(f"Log message {counter}")
        counter += 1

        # Emit error every 100 messages
        if EMIT_ERRORS and counter % 100 == 0:
            logger.error(f"Error message {counter}")

        # Emit multiline stack trace every 200 messages
        if EMIT_MULTILINE and counter % 200 == 0:
            logger.error("Exception occurred:\n" +
                "java.lang.NullPointerException\n" +
                "    at com.example.MyClass.method(MyClass.java:42)\n" +
                "    at com.example.Application.main(Application.java:15)")

        time.sleep(1.0 / LOG_RATE)

@app.route('/health')
def health():
    return 'OK', 200

if __name__ == '__main__':
    # Start log emission in background
    import threading
    log_thread = threading.Thread(target=emit_logs, daemon=True)
    log_thread.start()

    # Start HTTP server
    app.run(host='0.0.0.0', port=8080)
```

## Usage in Tests

Import and use in E2E tests:

```go
import (
    ecsloggenerator "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/ecs-log-generator"
)

// For EC2
workload, err := ecsloggenerator.EcsAppDefinition(env, clusterArn)

// For Fargate
workload, err := ecsloggenerator.FargateAppDefinition(env, clusterArn, apiKeySSM, fakeIntake)
```

Then validate in tests:

```go
// Validate log collection
logs, _ := fakeintake.GetLogs()
// Assert: logs contain expected messages
// Assert: logs have container metadata tags
// Assert: JSON logs are properly parsed

// Validate multiline handling
stackTraceLogs := filterLogsContaining(logs, "java.lang.NullPointerException")
// Assert: multiline logs are grouped together
// Assert: stack trace is not split across multiple log entries

// Validate log filtering
errorLogs := filterLogsByStatus(logs, "error")
// Assert: only ERROR level logs are included

// Validate trace correlation
logsWithTraceID := filterLogsWithTag(logs, "dd.trace_id")
// Assert: logs contain trace_id tags
// Assert: trace_ids match corresponding traces in fakeintake
```

## Test Coverage

This application is used by:

- `test/new-e2e/tests/containers/ecs_logs_test.go`
  - Test00AgentLogsReady
  - TestContainerLogCollection
  - TestLogMultiline
  - TestLogParsing
  - TestLogSampling
  - TestLogFiltering
  - TestLogSourceDetection
  - TestLogStatusRemapping
  - TestLogTraceCorrelation

## Maintenance

**Owned by**: ecs-experiences Team
**Purpose**: Test Infrastructure
**Used for**: ECS E2E Testing

### When to Update

- When adding new log collection features to test
- When log processing rules change
- When testing new log parsing capabilities
- When validating log pipeline performance improvements

### Do NOT Use For

- Production workloads
- Log management product testing (use dedicated Logs team test apps)
- Performance benchmarking
- Load testing

## Related Documentation

- [ECS E2E Testing Plan](../../../../../../../../CLAUDE.md)
- [E2E Testing Framework](../../../../README.md)
- [ECS Test Infrastructure](../../../../../../../test-infra-definition/)

## FAQ

**Q: Why is this owned by ecs-experiences team and not Logs team?**
A: This is infrastructure for testing how the **agent** collects logs in **ECS environments**. It's about validating agent functionality, not log management product features.

**Q: Can I use this for testing Logs product features?**
A: No. This is specifically for testing agent behavior in ECS. Use Logs-owned test applications for log management product feature testing.

**Q: Why emit multiple log types in one app instead of separate apps?**
A: It's more efficient for E2E tests to validate multiple log scenarios with a single deployment. Configuration via environment variables allows tests to control behavior dynamically.

**Q: What about other platforms (Kubernetes, Docker)?**
A: This app is ECS-specific due to ECS metadata enrichment and container lifecycle patterns. Similar apps should be created for other platforms (e.g., `k8s-log-generator` for Kubernetes).
