# Host Profiler Component

The `host-profiler` component implements the core profiling functionality for the Datadog Host Profiler, built on the OpenTelemetry Collector framework with eBPF-based continuous profiling capabilities.

## High-Level Architecture

This component wraps the OpenTelemetry Collector with custom receivers, processors, and exporters tailored for host-level continuous profiling. It leverages the OpenTelemetry eBPF Profiler to collect system-wide performance data with minimal overhead.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Host Profiler Component                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │          OpenTelemetry Collector Core                 │  │
│  │                                                       │  │
│  │  ┌──────────────┐    ┌──────────────┐    ┌─────────┐  │  │
│  │  │   Receiver   │───▶│  Processors  │───▶│Exporters│  │  │
│  │  │ (eBPF Based) │    │  (Enrichment)│    │ (OTLP)  │  │  │
│  │  └──────────────┘    └──────────────┘    └─────────┘  │  │
│  │                                                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────┐       ┌─────────────────────────────┐  │
│  │  Extra Factories│       │   Symbol Uploader           │  │
│  │ (Agent/No Agent)│       │   (Executable Reporting)    │  │
│  └─────────────────┘       └─────────────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Operational Modes

The component supports two operational modes, determined by the availability of Agent Core components:

1. **With Agent Core** (`ExtraFactoriesWithAgentCore`)
   - Infrastructure attributes processor (uses Agent tagger)
   - DD Profiling extension (uses Agent trace component)
   - Agent flare integration for diagnostic bundle generation and troubleshooting
   - Go runtime metrics and instrumentation (uses `instrumentation/runtime` for runtime stats)
   
2. **Without Agent Core** (`ExtraFactoriesWithoutAgentCore`)
   - K8s attributes processor (standalone Kubernetes metadata)
   - Configuration converters to remove Agent-dependent components
   - Simplified deployment without Agent dependencies

### Data Flow

```
Linux Kernel (eBPF Probes)
       ↓
Host Profiler Receiver
  - Collects CPU samples
  - Captures stack traces
  - Gathers process metadata
       ↓
Processors
  - Attributes processor (enrichment)
  - Infra attributes (Agent tags) OR K8s attributes
       ↓
Exporters
  - Debug (console logging)
  - OTLP HTTP (Datadog intake endpoint)
       ↓
Datadog Platform
```

## Directory Structure

### `collector/impl/`

- **[`collector.go`](collector/impl/collector.go)** - Implements the collector component by wrapping an OpenTelemetry Collector instance with custom configuration.
- **[`otel_col_factories.go`](collector/impl/otel_col_factories.go)** - Defines factory interfaces and implementations for creating OpenTelemetry components (receivers, processors, exporters, extensions) in both Agent and non-Agent modes.

#### `collector/impl/converters/`

- **[`converters.go`](collector/impl/converters/converters.go)** - Implements configuration converters that remove Agent-dependent components from the pipeline when running without Agent Core.

#### `collector/impl/receiver/`

- **[`factory.go`](collector/impl/receiver/factory.go)** - Creates the factory for the custom `hostprofiler` receiver, which builds the eBPF-based profiles receiver.
- **[`config.go`](collector/impl/receiver/config.go)** - Defines configuration structures for the receiver including eBPF collector settings, symbol uploader options, and validation logic.
- **[`executable_reporter.go`](collector/impl/receiver/executable_reporter.go)** - Implements the executable reporter that uploads debug symbols to Datadog for native code symbolization.
