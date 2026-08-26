# JMX Configuration Discovery PoC

## Overview

This document describes a proof-of-concept implementation of configuration
discovery for JMX integrations in the Datadog Agent. The PoC enables
auto-detection of JMX applications (e.g., Kafka, ActiveMQ, Tomcat) without
requiring users to manually write JMX integration configuration files with
explicit `conf` sections listing MBean domains and attributes.

## Table of Contents

1. [Architecture: Agent ↔ JMXFetch Interface](#architecture-agent--jmxfetch-interface)
2. [Existing Config Discovery (Python path)](#existing-config-discovery-python-path)
3. [The Gap: JMX Integrations](#the-gap-jmx-integrations)
4. [Implementation](#implementation)
   - [Agent Side: JMX Discovery Bridge](#agent-side-jmx-discovery-bridge)
   - [JMXFetch Side: MBean Inspection](#jmxfetch-side-mbean-inspection)
5. [Test Setup 1: Baseline (Manual Config)](#test-setup-1-baseline-manual-config)
6. [Test Setup 2: Discovery (Auto-Detection)](#test-setup-2-discovery-auto-detection)
7. [Results Comparison](#results-comparison)
8. [File Inventory](#file-inventory)

---

## Architecture: Agent ↔ JMXFetch Interface

The Datadog Agent communicates with JMXFetch (a Java process) via an HTTPS IPC
channel. The flow is:

```
┌─────────────────────────────────────────────────────┐
│  Core Agent (Go)                                     │
│                                                      │
│  ┌──────────────┐    ┌──────────────┐               │
│  │ Autodiscovery │    │  JmxScheduler │              │
│  │  (AutoConfig) │───▶│  (scheduler.go)│              │
│  └──────────────┘    └──────┬───────┘               │
│                             │                        │
│  ┌──────────────┐           │                        │
│  │  jmxfetch/   │           ▼                        │
│  │  state.go    │    ┌──────────────┐               │
│  │  GetIntegra- │    │  JMXFetch     │               │
│  │  tions()     │    │  (Java proc)  │               │
│  └──────┬───────┘    └──────▲───────┘               │
│         │                   │                        │
│         │  GET /agent/jmx/configs?timestamp=<ts>     │
│         │ ──────────────────▶                       │
│         │                   │                        │
│         │   JSON response   │                        │
│         │ ◀─────────────────                        │
│         │                   │                        │
│  ┌──────▼───────┐           │                        │
│  │  HTTP Handler │           │                        │
│  │  agent_jmx.go │           │                        │
│  └──────────────┘           │                        │
│                             │                        │
│  Agent starts JMXFetch as a subprocess with:          │
│  java -classpath jmxfetch.jar org.datadog.jmxfetch.App│
│    --ipc_host <host> --ipc_port <port>                │
│    --reporter statsd:... collect                      │
│                                                      │
└─────────────────────────────────────────────────────┘
```

### Key components:

- **`pkg/jmxfetch/scheduler.go`** — `JmxScheduler` receives AD integration
  configs, filters for JMX instances (`check.IsJMXInstance`), and schedules
  them via `state.scheduleConfig()`.
- **`pkg/jmxfetch/state.go`** — Maintains the scheduled configs cache.
  `GetIntegrations()` serializes all scheduled JMX configs as JSON for
  JMXFetch to poll.
- **`comp/api/api/apiimpl/internal/agent/agent_jmx.go`** — HTTP handler for
  `GET /agent/jmx/configs` endpoint. Returns JSON of scheduled JMX configs.
- **`pkg/jmxfetch/jmxfetch.go`** — Starts the JMXFetch Java subprocess with
  IPC host/port, reporter, and other options.
- **JMXFetch `HttpClient.java`** — Polls the agent's IPC endpoint for config
  updates every check period.
- **JMXFetch `App.java`** — Main loop: polls for configs, initializes
  instances, collects metrics.

### Config flow:

1. Autodiscovery discovers a container/service and resolves config templates
2. `JmxScheduler.Schedule()` receives resolved configs, filters for JMX
3. Configs are stored in `state.configs` cache
4. JMXFetch polls `GET /agent/jmx/configs?timestamp=<ts>` via HTTPS
5. Agent responds with JSON of all scheduled JMX configs
6. JMXFetch creates `Instance` objects from configs and connects to JMX endpoints

---

## Existing Config Discovery (Python path)

The Agent already has a configuration discovery mechanism for Python-based
integrations. The flow is:

```
Config template with `discovery: {}` field
        │
        ▼
  resolveTemplateForService()
        │  (skips normal resolution, enqueues discovery probe)
        ▼
  discoverer.Worker.Enqueue(svcID, tplDigest, integrationName)
        │
        ▼
  ConfigDiscoverer.DiscoverConfig(integrationName, serviceJSON)
        │  (calls Python rtloader: python.DiscoverConfig)
        ▼
  Python integration returns JSON with instances/init_config
        │
        ▼
  parseDiscoveryResult() → integration.Config
        │
        ▼
  configresolver.Resolve() → schedule resolved config
```

### Key types:

- **`integration.Config.Discovery`** — `*DiscoveryConfig` field; when non-nil,
  the config is a discovery template that skips normal template variable
  substitution.
- **`discoverer.ConfigDiscoverer`** interface — `DiscoverConfig(name, json)
  → (json, error)`
- **`discoverer.python_bridge.go`** — Calls Python rtloader's
  `discover_config()` C function
- **`discoverer.worker.go`** — Workqueue-backed async worker with retry logic

---

## The Gap: JMX Integrations

JMX integrations (Kafka, ActiveMQ, Tomcat, Cassandra, Solr) run through
JMXFetch (Java), not Python. The Python discovery bridge cannot handle JMX
integrations because:

1. JMX integrations are not Python checks — they're handled by JMXFetch
2. The Python `discover_config()` function calls into the Python check's
   `discover()` method, which JMX integrations don't have
3. JMXFetch needs to connect to the JMX endpoint to inspect MBeans, which
   can't be done from Python

The solution: add a JMX discovery bridge that generates basic JMX instance
configs (host/port), and let JMXFetch inspect MBeans at runtime to auto-detect
the application type and configure metric collection.

---

## Implementation

### Agent Side: JMX Discovery Bridge

#### New files:

**`comp/core/autodiscovery/discoverer/jmx_bridge.go`** (`//go:build jmx`)

Implements `ConfigDiscoverer` for JMX integrations. When a discovery template
for a JMX integration (e.g., `kafka`) is enqueued, the JMX bridge:

1. Parses the service JSON to get host and ports
2. Finds a JMX port (by name "jmx"/"jmx-rmi", or common JMX ports like 9999,
   9010, 1099, 7199)
3. Returns a discovered config with:
   - `host: "%%host%%"` (will be resolved by configresolver)
   - `port: <jmx_port>`
   - `collect_default_jvm_metrics: true`
   - `discovery: true` (flag for JMXFetch to auto-detect)
   - `init_config` with `is_jmx: true`, `collect_default_metrics: true`

**`comp/core/autodiscovery/discoverer/jmx_bridge_nojmx.go`** (`//go:build !jmx`)

Returns nil when agent is built without JMX support.

**`comp/core/autodiscovery/discoverer/composite.go`**

Routes discovery probes to the appropriate bridge:
- JMX integrations (identified by `check.StandardJMXIntegrations`) → JMX bridge
- All others → Python bridge

#### Modified files:

**`comp/core/autodiscovery/impl/autoconfig.go`**

Changed the `newReconcilingConfigManager` call to use
`NewCompositeDiscoverer(NewPythonBridge(), NewJmxBridge())` instead of just
`NewPythonBridge()`.

### JMXFetch Side: MBean Inspection

#### New files:

**`src/main/java/org/datadog/jmxfetch/JmxDiscovery.java`**

Inspects MBean domains on a JMX connection to auto-detect the application
type. Contains a list of `AppSignature` entries that map MBean domain
patterns to conf entries:

| Application | Required Domains | Conf Entries |
|---|---|---|
| kafka | kafka.server, kafka.controller | include: domain=kafka.server, include: domain=kafka.controller |
| activemq | org.apache.activemq | include: domain=org.apache.activemq |
| tomcat | Tomcat | include: domain=Tomcat |
| cassandra | org.apache.cassandra | include: domain=org.apache.cassandra |
| solr | solr | include: domain=solr |

The `discover(Connection)` method:
1. Gets `MBeanServerConnection` from the JMX connection
2. Queries all MBeans (`*:*`) and collects domain names
3. Matches domains against signatures
4. Returns conf entries for the first matching application type

#### Modified files:

**`src/main/java/org/datadog/jmxfetch/Connection.java`**

Added `getMBeanServerConnection()` method to expose the underlying
`MBeanServerConnection` for discovery inspection.

**`src/main/java/org/datadog/jmxfetch/Instance.java`**

Modified `init(boolean forceNewConnection)` to check for the `discovery: true`
flag. When set and no explicit `conf` section was provided, calls
`JmxDiscovery.discover(connection)` to auto-detect the application type and
adds the discovered conf entries to `configurationList`.

Key logic:
```java
Boolean discoveryEnabled = (Boolean) instanceMap.get("discovery");
boolean hasExplicitConf = instanceMap.containsKey("conf")
        || (initConfig != null && initConfig.containsKey("conf"));
if (discoveryEnabled != null && discoveryEnabled && !hasExplicitConf) {
    List<Map<String, Object>> discoveredConf = JmxDiscovery.discover(connection);
    // ... add discovered conf entries
}
```

---

## Test Setup 1: Baseline (Manual Config)

### Objective

Verify the existing Agent ↔ JMXFetch interface works with Kafka using
traditional manual JMX integration configuration.

### Docker Compose

File: `docker-compose-baseline.yaml`

```yaml
services:
  kafka:
    image: confluentinc/cp-kafka:7.7.0
    environment:
      KAFKA_JMX_PORT: 9999
      KAFKA_JMX_HOSTNAME: kafka
      KAFKA_JMX_OPTS: >
        -Dcom.sun.management.jmxremote
        -Dcom.sun.management.jmxremote.port=9999
        -Dcom.sun.management.jmxremote.rmi.port=9999
        -Dcom.sun.management.jmxremote.authenticate=false
        -Dcom.sun.management.jmxremote.ssl=false
        -Djava.rmi.server.hostname=kafka
    labels:
      com.datadoghq.ad.checks: |
        {
          "kafka": {
            "init_config": {
              "is_jmx": true,
              "collect_default_metrics": true,
              "new_gc_metrics": true
            },
            "instances": [
              { "host": "%%host%%", "port": "9999" }
            ]
          }
        }

  datadog:
    image: datadog/agent-dev:nightly-main-py3-jmx
    environment:
      DD_API_KEY: "000000000000000001"
      DD_JMX_TELEMETRY_ENABLED: "true"
      DD_LOG_LEVEL: "debug"
```

### How to run

```bash
cd ~/dd/jmx-discovery-poc
docker compose -f docker-compose-baseline.yaml up -d
# Wait ~60 seconds for Kafka to start and agent to discover it
docker logs dd-agent-baseline 2>&1 | grep "sending.*metric"
```

### Expected output

```
Instance kafka-172.17.130.4-9999 is sending 28 metrics to the metrics reporter during collection #5
```

The baseline collects **28 metrics** per collection cycle — only default JVM
metrics, since the manual config doesn't include a `conf` section with
Kafka-specific MBean filters.

---

## Test Setup 2: Discovery (Auto-Detection)

### Objective

Verify that JMX configuration discovery works: the agent uses an AD template
with `ad_identifiers` to match the Kafka container, and JMXFetch inspects
MBeans to auto-detect Kafka and collect Kafka-specific metrics.

### Prerequisites

Build local Docker images with the modified jmxfetch:

```bash
# Build jmxfetch jar
cd ~/dd/jmxfetch && ./mvnw -DskipTests clean package assembly:single

# Build local agent-discovery image (stock agent + modified jmxfetch jar)
cp ~/dd/jmxfetch/target/jmxfetch-*-jar-with-dependencies.jar ~/dd/jmx-discovery-poc/
cd ~/dd/jmx-discovery-poc
docker build -f Dockerfile.agent-discovery -t dd-agent-discovery:dev .
```

### Config file

File: `conf.d/kafka.d/conf.yaml`

```yaml
ad_identifiers:
  - cp-kafka

init_config:
  is_jmx: true
  collect_default_metrics: true
  new_gc_metrics: true

instances:
  - host: "%%host%%"
    port: 9999
    discovery: true
```

Key differences from baseline:
- **No Docker labels** with `com.datadoghq.ad.checks` — config comes from
  the conf.d file
- **`ad_identifiers: [cp-kafka]`** — matches the Kafka container by its
  short image name
- **`discovery: true`** in the instance — tells JMXFetch to auto-detect the
  application type by inspecting MBeans
- **No `conf` section** — JMXFetch will generate the conf entries at runtime

### Docker Compose

File: `docker-compose-discovery.yaml`

```yaml
services:
  kafka:
    image: confluentinc/cp-kafka:7.7.0
    # Same JMX setup as baseline, but NO com.datadoghq.ad.checks labels
    environment:
      KAFKA_JMX_PORT: 9999
      KAFKA_JMX_HOSTNAME: kafka
      KAFKA_JMX_OPTS: >
        -Dcom.sun.management.jmxremote
        -Dcom.sun.management.jmxremote.port=9999
        ...

  datadog:
    image: dd-agent-discovery:dev  # Local image with modified jmxfetch
    volumes:
      - ./conf.d:/etc/datadog-agent/conf.d:ro  # Mount discovery config
```

### How to run

```bash
cd ~/dd/jmx-discovery-poc
docker compose -f docker-compose-discovery.yaml up -d
# Wait ~75 seconds for Kafka to start and discovery to work
docker logs dd-agent-discovery 2>&1 | grep -E "discovery|sending.*metric"
```

### Expected output

```
Configuration discovery enabled for instance kafka-172.17.130.4-9999, inspecting MBeans to auto-detect application type...
JMX discovery: found MBean domains: [java.util.logging, jdk.management.jfr, kafka.controller, kafka.utils, java.nio, kafka.network, kafka.log, JMImplementation, kafka.coordinator.group, java.lang, kafka.server, com.sun.management, kafka, kafka.coordinator.transaction]
JMX discovery: detected application type: kafka
Configuration discovery: added 2 conf entries for instance kafka-172.17.130.4-9999
Instance kafka-172.17.130.4-9999 is sending 350 metrics to the metrics reporter during collection #3
```

The discovery test collects **350 metrics** per collection cycle — 12.5x more
than the baseline — because JMXFetch auto-detected Kafka and added conf entries
for `kafka.server` and `kafka.controller` MBean domains.

---

## Results Comparison

| Metric | Baseline (Manual) | Discovery (Auto) |
|---|---|---|
| Config source | Docker AD labels | conf.d template + AD identifiers |
| `conf` section | Not provided | Auto-generated by JMXFetch |
| Metrics per cycle | 28 | 350 |
| Application detection | Manual (user knows it's Kafka) | Auto (JMXFetch inspects MBeans) |
| User config required | Full JMX config in labels | Minimal template with `discovery: true` |

---

## File Inventory

### Agent changes (`~/dd/datadog-agent/`)

| File | Status | Description |
|---|---|---|
| `comp/core/autodiscovery/discoverer/jmx_bridge.go` | New | JMX discovery bridge (build: jmx) |
| `comp/core/autodiscovery/discoverer/jmx_bridge_nojmx.go` | New | No-op for non-JMX builds |
| `comp/core/autodiscovery/discoverer/composite.go` | New | Routes discovery to Python or JMX bridge |
| `comp/core/autodiscovery/impl/autoconfig.go` | Modified | Uses composite discoverer |

### JMXFetch changes (`~/dd/jmxfetch/`)

| File | Status | Description |
|---|---|---|
| `src/main/java/org/datadog/jmxfetch/JmxDiscovery.java` | New | MBean inspection and app type detection |
| `src/main/java/org/datadog/jmxfetch/Connection.java` | Modified | Added `getMBeanServerConnection()` |
| `src/main/java/org/datadog/jmxfetch/Instance.java` | Modified | Discovery logic in `init()` |

### PoC files (`~/dd/jmx-discovery-poc/`)

| File | Description |
|---|---|
| `docker-compose-baseline.yaml` | Baseline test with manual JMX config |
| `docker-compose-discovery.yaml` | Discovery test with auto-detection |
| `conf.d/kafka.d/conf.yaml` | AD template with `discovery: true` |
| `Dockerfile.agent-discovery` | Builds image with modified jmxfetch jar |
| `Dockerfile.jmxfetch` | Builds jmxfetch-only image |
| `Dockerfile.agent` | Builds agent + jmxfetch image |

---

## References

- [Configuration Discovery for Agent Integrations](https://datadoghq.atlassian.net/wiki/spaces/DSCVR/pages/6671862234/Configuration+Discovery+for+Agent+Integrations)
- [The Quest for mBeans: A JMX Adventure](https://datadoghq.atlassian.net/wiki/spaces/TS/pages/4042589782/The+Quest+for+mBeans+A+JMX+Adventure)
