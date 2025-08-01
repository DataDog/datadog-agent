# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Essential Commands

This project uses the `dda` command (Datadog Agent CLI) powered by Python's invoke framework for all build operations:

### Build Commands
- **Build agent**: `dda inv agent.build`
- **Modular build**: `dda inv agent.build --build-exclude=systemd,python`
- **Build specific agents**:
  - `dda inv cluster-agent.build`
  - `dda inv dogstatsd.build` 
  - `dda inv trace-agent.build`
  - `dda inv system-probe.build`
- **Install development tools**: `dda inv install-tools`

### Testing Commands
- **Run tests**: `dda inv test`
- **Module-specific tests**: `dda inv test --module=pkg/trace --targets=./api,./config`
- **Tests with race detection**: `dda inv test --race`
- **Tests with coverage**: `dda inv test --coverage`
- **Single package test**: `dda inv test --targets=./pkg/specific/package`

### Linting Commands
- **Go linting**: `dda inv linter.go`
- **All linting**: `dda inv linter.go linter.python linter.shell`

## Architecture Overview

### Multi-Agent System
The Datadog Agent is composed of several specialized agents:
- **Core Agent** (`cmd/agent/`) - Metrics, logs, APM, integrations
- **Cluster Agent** (`cmd/cluster-agent/`) - Kubernetes cluster monitoring
- **Process Agent** (`cmd/process-agent/`) - Process and container monitoring
- **System Probe** (`cmd/system-probe/`) - Network and security via eBPF
- **Trace Agent** (`cmd/trace-agent/`) - APM trace processing
- **DogStatsD** (`cmd/dogstatsd/`) - High-performance metrics aggregation

### Code Organization Patterns

**Component Architecture** (`comp/`): New dependency injection-based components organized by function:
- `comp/core/` - Configuration, logging, secrets, telemetry
- `comp/aggregator/` - Metrics aggregation
- `comp/forwarder/` - Data forwarding to Datadog
- `comp/logs/` - Log collection and processing
- `comp/trace/` - APM trace processing

**Legacy Packages** (`pkg/`): Shared libraries and utilities:
- `pkg/collector/` - Check execution and scheduling
- `pkg/config/` - Configuration management
- `pkg/tagger/` - Entity tagging system
- `pkg/metrics/` - Metrics types and processing
- `pkg/network/` - Network monitoring components
- `pkg/security/` - Runtime security monitoring
- `pkg/ebpf/` - eBPF program management

### Multi-Module Go Workspace
Uses Go workspaces (`go.work`) with 170+ modules defined in `modules.yml`. Each module has specific build targets and test configurations.

## Development Workflow

### Setup Requirements
- Go 1.24+
- Python 3.12+ with `pip install dda`
- CMake 3.15+ and C++ compiler for native components

### Modular Development
Use `--build-include` and `--build-exclude` flags for faster iteration:
```bash
# Exclude heavy components during development
dda inv agent.build --build-exclude=systemd,python,docker
```

Available components: `apm`, `python`, `docker`, `jmx`, `log`, `process`, `systemd`, `secrets`, `kubeapiserver`, `kubelet`, `orchestrator`

### Configuration
Create `dev/dist/datadog.yaml` with minimal config:
```yaml
api_key: your_key_here
log_level: debug
```

## Key Technical Details

### Testing Infrastructure
- Standard Go testing with `testing` package
- Platform-specific test conditions (Linux, Windows, macOS)
- E2E tests in `test/new-e2e/` using Pulumi-managed environments
- Flake detection managed via `flakes.yaml`

### Code Quality
- **Go**: golangci-lint (configured in `.golangci.yml`)
- **Python**: ruff + mypy (configured in `pyproject.toml`)
- **Shell**: shellcheck

### Build System
The `dda` command provides extensive build customization and is the primary interface for all development tasks. Avoid using raw `go build` or `make` commands directly.