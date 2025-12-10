# ECS Chaos Test Application

## Overview

The ECS Chaos test application is a **test infrastructure component** owned by the **ecs-experiences team** for validating agent resilience and error handling in ECS environments.

## Purpose

This application exists to test and validate:

1. **Agent Restart Recovery**: Agent gracefully handles restarts and resumes data collection
2. **Task Failure Handling**: Agent properly handles task failures and replacements
3. **Network Resilience**: Agent buffers and retries during network interruptions
4. **High Cardinality**: Agent handles high cardinality metrics without memory issues
5. **Resource Exhaustion**: Agent degrades gracefully under low memory/CPU conditions
6. **Container Churn**: Agent handles rapid container creation/deletion without leaks
7. **Large Payloads**: Agent chunks and handles large traces/logs without truncation
8. **Backpressure Handling**: Agent buffers data when downstream is slow

## Architecture

The chaos application is a configurable service that simulates failure scenarios:

```
┌─────────────────────┐
│   Chaos App         │
│  (Configurable)     │
│                     │
│  • Memory Leak      │
│  • CPU Spike        │
│  • Crash/Restart    │
│  • High Cardinality │
│  • Network Timeout  │
│  • Large Payloads   │
└─────────────────────┘
         │
         ▼
  Datadog Agent
  (Under Stress)
         │
         ▼
   FakeIntake
```

## Configuration

The chaos app is controlled via environment variables:

```bash
# Chaos mode selection
CHAOS_MODE=normal              # normal, memory_leak, cpu_spike, crash,
                               # high_cardinality, network_timeout, large_payload

# Memory leak simulation
MEMORY_LEAK_RATE=1             # MB per second to allocate

# CPU spike simulation
CPU_SPIKE_INTERVAL=60          # seconds between CPU spikes

# Crash simulation
CRASH_INTERVAL=300             # seconds between crashes (0 = disabled)

# High cardinality simulation
HIGH_CARDINALITY_TAGS=100      # number of unique tag combinations

# Metric emission
METRIC_EMISSION_RATE=10        # metrics per second

# Large payload simulation
LARGE_PAYLOAD_SIZE=0           # KB per trace/log (0 = normal)

# Network timeout simulation
NETWORK_TIMEOUT_RATE=0         # percentage of requests that timeout (0-100)

# Datadog configuration
DD_SERVICE=chaos
DD_ENV=test
DD_VERSION=1.0
DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket
DD_LOGS_INJECTION=true
```

## Chaos Modes

### 1. Normal Mode (`CHAOS_MODE=normal`)
- Emits regular metrics, logs, and traces
- No stress or failures
- Baseline for comparison

### 2. Memory Leak Mode (`CHAOS_MODE=memory_leak`)
- Gradually allocates memory at configured rate
- Does not release allocated memory
- Tests agent behavior under memory pressure
- Use: Validate agent doesn't crash when app has memory leak

### 3. CPU Spike Mode (`CHAOS_MODE=cpu_spike`)
- Periodically spikes CPU usage to 100%
- Duration: 10-30 seconds per spike
- Use: Validate agent continues collecting during CPU contention

### 4. Crash Mode (`CHAOS_MODE=crash`)
- Randomly crashes and restarts
- Interval configured by `CRASH_INTERVAL`
- Use: Validate agent handles container restarts gracefully

### 5. High Cardinality Mode (`CHAOS_MODE=high_cardinality`)
- Emits metrics with many unique tag combinations
- Number of unique tags: `HIGH_CARDINALITY_TAGS`
- Use: Validate agent memory doesn't explode with high cardinality

### 6. Network Timeout Mode (`CHAOS_MODE=network_timeout`)
- Simulates slow/failing network requests
- Percentage of failures: `NETWORK_TIMEOUT_RATE`
- Use: Validate agent buffers and retries properly

### 7. Large Payload Mode (`CHAOS_MODE=large_payload`)
- Emits large traces and logs
- Size: `LARGE_PAYLOAD_SIZE` KB
- Use: Validate agent chunks and handles large data

## Docker Image

The application requires the Docker image:

- `ghcr.io/datadog/apps-ecs-chaos:<version>`

### Image Requirements

The image should:
- Support all chaos modes via environment variables
- Emit metrics, logs, and traces to Datadog agent
- Include health check endpoint (HTTP server on port 8080)
- Handle crashes and restarts gracefully (when in crash mode)
- Generate realistic high-cardinality data

### Example Implementation (Python)

```python
import os
import time
import random
import threading
import traceback
from flask import Flask
from ddtrace import tracer, patch_all
import logging

patch_all()
app = Flask(__name__)

# Configuration
CHAOS_MODE = os.getenv('CHAOS_MODE', 'normal')
MEMORY_LEAK_RATE = int(os.getenv('MEMORY_LEAK_RATE', '1'))
CPU_SPIKE_INTERVAL = int(os.getenv('CPU_SPIKE_INTERVAL', '60'))
CRASH_INTERVAL = int(os.getenv('CRASH_INTERVAL', '0'))
HIGH_CARDINALITY_TAGS = int(os.getenv('HIGH_CARDINALITY_TAGS', '100'))
METRIC_EMISSION_RATE = int(os.getenv('METRIC_EMISSION_RATE', '10'))

# Memory leak storage
leaked_memory = []

def memory_leak_worker():
    """Gradually leak memory"""
    while CHAOS_MODE == 'memory_leak':
        # Allocate 1MB chunks
        leaked_memory.append(bytearray(1024 * 1024 * MEMORY_LEAK_RATE))
        time.sleep(1)
        logging.info(f"Leaked memory: {len(leaked_memory)} MB")

def cpu_spike_worker():
    """Periodically spike CPU"""
    while CHAOS_MODE == 'cpu_spike':
        time.sleep(CPU_SPIKE_INTERVAL)
        logging.warning("Starting CPU spike")
        end_time = time.time() + random.uniform(10, 30)
        while time.time() < end_time:
            # Busy loop
            _ = sum(range(1000000))
        logging.info("CPU spike complete")

def crash_worker():
    """Randomly crash"""
    if CRASH_INTERVAL > 0:
        time.sleep(CRASH_INTERVAL + random.uniform(-30, 30))
        logging.error("Simulated crash!")
        os._exit(1)

def emit_metrics_worker():
    """Emit metrics continuously"""
    from datadog import initialize, statsd
    initialize()

    counter = 0
    while True:
        if CHAOS_MODE == 'high_cardinality':
            # Emit with unique tags
            tag = f"unique_id:{counter % HIGH_CARDINALITY_TAGS}"
            statsd.increment('chaos.metric', tags=[tag])
        else:
            statsd.increment('chaos.metric')

        counter += 1
        time.sleep(1.0 / METRIC_EMISSION_RATE)

@app.route('/health')
def health():
    return 'OK', 200

@app.route('/')
def index():
    # Emit trace
    with tracer.trace('chaos.request'):
        logging.info(f"Request handled in {CHAOS_MODE} mode")
        return f'Chaos mode: {CHAOS_MODE}', 200

if __name__ == '__main__':
    # Start chaos workers
    if CHAOS_MODE == 'memory_leak':
        threading.Thread(target=memory_leak_worker, daemon=True).start()
    elif CHAOS_MODE == 'cpu_spike':
        threading.Thread(target=cpu_spike_worker, daemon=True).start()

    if CRASH_INTERVAL > 0:
        threading.Thread(target=crash_worker, daemon=True).start()

    # Start metric emission
    threading.Thread(target=emit_metrics_worker, daemon=True).start()

    # Start HTTP server
    app.run(host='0.0.0.0', port=8080)
```

## Usage in Tests

Import and use in E2E tests:

```go
import (
    ecschaos "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/ecs-chaos"
)

// For EC2
workload, err := ecschaos.EcsAppDefinition(env, clusterArn)
```

Then validate in tests:

```go
// Test agent restart recovery
// 1. Restart agent container
// 2. Wait for agent to come back up
// 3. Verify metrics resume flowing

// Test high cardinality handling
metrics, _ := fakeintake.GetMetrics()
uniqueTags := countUniqueTags(metrics)
// Assert: agent memory usage is reasonable
// Assert: all metrics are collected

// Test memory pressure
// 1. Enable memory leak mode
// 2. Wait for container to use significant memory
// 3. Verify agent still collects data
// Assert: agent doesn't crash
```

## Test Coverage

This application is used by:

- `test/new-e2e/tests/containers/ecs_resilience_test.go`
  - TestAgentRestart
  - TestTaskFailureRecovery
  - TestNetworkInterruption
  - TestHighCardinality
  - TestResourceExhaustion
  - TestRapidContainerChurn
  - TestLargePayloads
  - TestBackpressure

## Maintenance

**Owned by**: ecs-experiences Team
**Purpose**: Test Infrastructure
**Used for**: ECS E2E Testing - Resilience Validation

### When to Update

- When adding new failure scenarios to test
- When validating new agent resilience features
- When testing agent behavior under extreme conditions
- When reproducing production issues in test environment

### Do NOT Use For

- Production workloads
- Performance benchmarking
- Load testing
- Actual chaos engineering in production

## Related Documentation

- [ECS E2E Testing Plan](../../../../../../../../CLAUDE.md)
- [E2E Testing Framework](../../../../README.md)
- [ECS Test Infrastructure](../../../../../../../test-infra-definition/)

## FAQ

**Q: Why is this owned by ecs-experiences team?**
A: This tests **agent resilience** in ECS, not application resilience. It's infrastructure for validating how the agent handles failures.

**Q: Should I use this for actual chaos engineering?**
A: No. This is for testing the Datadog agent's resilience, not for chaos engineering in production systems.

**Q: Can I add new chaos modes?**
A: Yes! Add the mode to the CHAOS_MODE environment variable and implement the behavior in the Docker image.

**Q: Why only EC2 variant, not Fargate?**
A: Resilience testing focuses on agent behavior, which is consistent across deployment types. EC2 provides more control for testing scenarios like agent restarts.

**Q: How do I test network interruptions?**
A: Use the network timeout mode or use external tools (iptables, toxiproxy) to simulate network failures at the infrastructure level.
