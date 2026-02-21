# Host Profiler Command

The `host-profiler` command is the entry point for the standalone Datadog Host Profiler binary, which collects system-wide, low-overhead profiling data from Linux hosts using eBPF technology.

> [!WARNING]
> **⚠️ Linux Only**: This binary is **only supported on Linux** systems with eBPF capabilities (kernel 4.15+). It will not compile or run on macOS, Windows, or other operating systems.

## Building

To build the host-profiler binary, use the following invoke task:

```bash
dda inv full-host-profiler.build
```

> [!NOTE]
> This must be run on a Linux system or in a Linux build environment, as the binary includes Linux-specific eBPF dependencies.

## Testing

```bash
# Run all tests
dda inv test

# Run tests for a specific package
dda inv test --targets=./comp/host-profiler/collector/
```

## CLI Flags

### Global Flags

- `-c, --config` - Path to host-profiler configuration file (OpenTelemetry Collector format)
- `--core-config` - Path to Datadog Agent config file; enables Agent integration features (optional)

### Run Subcommand Flags

- `--sync-timeout` - Timeout for config sync requests (default: 3s)
- `--sync-on-init-timeout` - How long config sync should retry at initialization before failing (default: 0, disabled)

## Configuration

The component is configured via an OpenTelemetry Collector YAML file. See [`dist/host-profiler-config.yaml`](dist/host-profiler-config.yaml) for the default configuration.

### Key Configuration Sections

- **`receivers.hostprofiler`**: eBPF profiling parameters, tracers, symbol upload settings
- **`processors.infraattributes`**: Infrastructure metadata enrichment (Agent mode only)
- **`processors.k8sattributes`**: Kubernetes metadata enrichment (standalone mode)
- **`exporters.otlphttp`**: Datadog profiling intake endpoint configuration
- **`extensions.ddprofiling`**: Datadog profiling extension (Agent mode only)

### Configuration Inference

Configuration inference is enabled by default in bundled mode when symbol_endpoints and otlphttp exporters are not explicitly configured.

The host profiler searches for the following nodes in the core agent's configuration file:
- **`apm_config`**:
    - **`profiling_dd_url`**: URL from which the site is extracted (takes priority)
    - **`profiling_additional_endpoints`**: additional endpoints with a url to extract `site` and `n` number of api keys
- **`site`**: secondary site (if `profiling_dd_url` isn't configured) for symbol endpoints and otlphttp's `profiles_endpoint`
- **`api_key`**: main api key for `site`

This inference will create as many symbol endpoints and otlphttp exporters as there are site+key combinations.

## Running

### Using docker-compose (recommended)

This is the recommended way to build and run the host profiler for development.

Create a `.env` file in `cmd/host-profiler` containing:

```
DD_SITE=datad0g.com # optional, defaults to "datadoghq.com"
UID=1234 # required on Datadog workspace, set to the output of `id -u` on the workspace
GID=1234 # required on Datadog workspace, set to the output of `id -g` on the workspace
```

> [!IMPORTANT]
> All docker-compose commands must be run from the `cmd/host-profiler` directory.

Then build and run with

```bash
docker-compose up --build
```

Check profiler's logs with

```bash
docker-compose logs host-profiler -f
```

Once the profiler is running, you can access the profiling dashboards on the site configured in `DD_SITE`.

### Running on Host (deprecated)

```bash
# Standalone mode
./bin/full-host-profiler/full-host-profiler run -c cmd/host-profiler/dist/host-profiler-config.yaml

# Agent-integrated mode
# First, start the Datadog Agent
./bin/agent/agent run -c ./dev/dist

# Then, in another terminal, start the host-profiler with Agent integration
./bin/full-host-profiler/full-host-profiler run \
  -c cmd/host-profiler/dist/host-profiler-config.yaml \
  --core-config ./dev/dist/datadog.yaml
```

> [!WARNING]
> In this configuration, container attributes might be broken.

## High-Level Architecture

The Host Profiler is built on top of the OpenTelemetry Collector architecture and leverages the OpenTelemetry eBPF Profiler to capture profiles across all processes on a Linux system.

### Architecture Modes

The binary dynamically selects its operational mode based on the presence of the `--core-config` flag. This is implemented using **two factory implementations** for OpenTelemetry components (`ExtraFactoriesWithAgentCore` and `ExtraFactoriesWithoutAgentCore` in [`otel_col_factories.go`](../../comp/host-profiler/collector/impl/otel_col_factories.go)):

1. **Standalone Mode** (without Agent Core)
   - **Triggered when**: `--core-config` is NOT provided
   - **Features**:
     - Basic profiling without Datadog Agent integration
     - Limited infrastructure metadata (K8s only, no Agent tagging)

2. **Agent-Integrated Mode** (with Agent Core)
   - **Triggered when**: `--core-config` is provided
   - **Features**:
     - Infrastructure attributes enrichment via the Agent's tagger
     - Leverages the Datadog profiling extension (`ddprofilingextension`) to capture and export detailed performance profiles for the entire Host Profiler process.
     - Agent flare integration for diagnostic bundle generation and troubleshooting
     - Go runtime metrics and instrumentation (uses `instrumentation/runtime` for runtime stats)

### Data Flow

```
eBPF Kernel Probes
       ↓
Host Profiler Receiver (OpenTelemetry eBPF)
       ↓
Processors (Attributes, Infrastructure Tags)
       ↓
Exporters (OTLP HTTP → Datadog Intake)
```

The profiler collects continuous profiling data from the Linux kernel via eBPF, processes it through OpenTelemetry pipelines, enriches it with metadata and tags, and exports it to Datadog's profiling intake endpoint.

### Component Integration

This command acts as the CLI wrapper that initializes and runs the core profiling component located in `comp/host-profiler`. For implementation details of the profiling logic, see [comp/host-profiler/README.md](../../comp/host-profiler/README.md).
