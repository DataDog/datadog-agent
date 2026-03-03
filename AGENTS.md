# Datadog Agent - Project Overview for AI coding assistant

## Project Summary
The Datadog Agent is a comprehensive monitoring and observability agent written primarily in Go. It collects metrics, traces, logs, and security events from systems and applications, forwarding them to the Datadog platform. This is the main repository for Agent versions 6 and 7.

## Project Structure

### Core Directories
- `/cmd/` - Entry points for various agent components
  - `agent/` - Main agent binary
  - `cluster-agent/` - Kubernetes cluster agent
  - `dogstatsd/` - StatsD metrics daemon
  - `trace-agent/` - APM trace collection agent
  - `system-probe/` - System-level monitoring (eBPF)
  - `security-agent/` - Security monitoring
  - `process-agent/` - Process monitoring
  - `privateactionrunner/` - Executing actions

- `/pkg/` - Core Go packages and libraries
  - `aggregator/` - Metrics aggregation
  - `collector/` - Check scheduling and execution
  - `config/` - Configuration management
  - `logs/` - Log collection and processing
  - `metrics/` - Metrics types and handling
  - `network/` - Network monitoring
  - `security/` - Security monitoring components
  - `trace/` - APM tracing components

- `/comp/` - Component-based architecture modules
  - `core/` - Core components
  - `metadata/` - Metadata collection
  - `logs/` - Log components
  - `trace/` - Trace components

- `/tasks/` - Python invoke tasks for development
  - Build, test, lint, and deployment automation

- `/rtloader/` - Runtime loader for Python checks

## Development Workflow

### Common Commands

#### Building

```bash
# install dda on mac OS
brew install --cask dda

# Install development tools
dda inv install-tools

# Build the main agent
dda inv agent.build --build-exclude=systemd

# Build specific components
dda inv dogstatsd.build
dda inv trace-agent.build
dda inv system-probe.build
```

#### Linting & Testing (dda inv)

**Go (lint & tests)**
- Lint all Go modules: `dda inv linter.go`
- Lint main module specific packages: `dda inv linter.go --module=. --targets=./pkg/collector/check,./pkg/aggregator`
- Lint a single Go module (its default lint targets): `dda inv linter.go --module=comp/core/log/impl`
- Test all Go modules: `dda inv test`
- Test main module specific packages: `dda inv test --module=. --targets=./pkg/aggregator`
- Test a single Go module: `dda inv test --module=comp/trace/compression/impl-zstd`
- Scope to changed code: `--only-modified-packages` = packages with direct file edits; `--only-impacted-packages` = modified packages plus dependents discovered from imports

**Python lint**
- Repository-wide lint (ruff, vulture, mypy): `dda inv linter.python`
- Show linter versions: `dda inv linter.python --show-versions`
- Note: `linter.python` runs on the full tree; it does not accept per-file or per-path targets.

**Python tests**
- All Python unit tests for invoke tasks: `dda inv invoke-unit-tests.run --directory=tasks/unit_tests`
- Single test module file (drop `_tests.py` suffix; e.g., `git` runs `git_tests.py`): `dda inv invoke-unit-tests.run --directory=tasks/unit_tests --tests=git`
- Multiple specific test modules: `dda inv invoke-unit-tests.run --directory=tasks/unit_tests --tests=git,github_tasks`
- rtloader tests (Python/C embedding): `dda inv rtloader.test`

#### Running Locally
```bash
# Create dev config with testing API key
echo "api_key: 0000001" > dev/dist/datadog.yaml

# Run the agent
./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

### Development Configuration
The development configuration file should be placed at `dev/dist/datadog.yaml`. After building, it gets copied to `bin/agent/dist/datadog.yaml`.

## Key Components

### Check System
- Checks are Python or Go modules that collect metrics
- Located in `cmd/agent/dist/checks/`
- Can be autodiscovered via Kubernetes annotations/labels

### Configuration
- Main config: `datadog.yaml`
- Check configs: `conf.d/<check_name>.d/conf.yaml`
- Supports environment variable overrides with `DD_` prefix

### eBPF-based System Checks
- Checks using eBPF probes require system-probe module running
- Examples: tcp_queue_length, oom_kill, seccomp_tracer
- Module code (system-probe): `pkg/collector/corechecks/ebpf/probe/<check>/`
- Check code (agent): `pkg/collector/corechecks/ebpf/<check>/`
- System-probe modules: `cmd/system-probe/modules/`
- Configuration: Set `<check_name>.enabled: true` in system-probe config
- See `pkg/collector/corechecks/ebpf/AGENTS.md` for detailed structure
- Quick reference: `.cursor/rules/system_probe_modules.mdc` for common patterns and pitfalls

## Testing Strategy

### Unit Tests
- Go tests using standard `go test`
- Python tests using pytest
- Run with `dda inv test --targets=<package>`

### End-to-End Tests
- E2E framework in `test/new-e2e/`

### Linting
- Go: golangci-lint via `dda inv linter.go`
- Python: various linters via `dda inv linter.python`
- YAML: yamllint
- Shell: shellcheck

## Build System

### Invoke Tasks
The project uses Python's Invoke framework with custom tasks. Main task categories:
- `agent.*` - Core agent tasks
- `test` - Testing tasks
- `linter.*` - Linting tasks
- `docker.*` - Docker image tasks
- `release.*` - Release management

### Build Tags
Go build tags control feature inclusion, some examples are:
- `kubeapiserver` - Kubernetes API server support
- `containerd` - containerd support
- `docker` - Docker support
- `ebpf` - eBPF support
- `python` - Python check support
- and MANY more, refer to ./tasks/build_tags.py for a full reference.

## Important Files

### Configuration
- `datadog.yaml` - Main agent configuration
- `modules.yml` - Go module definitions
- `release.json` - Release version information
- `.gitlab-ci.yml` - CI/CD pipeline configuration

### Documentation
- `/docs/` - Internal documentation
- `/docs/dev/` - Developer guides
- `README.md` - Project overview
- `CONTRIBUTING.md` - Contribution guidelines

## CI/CD Pipeline

### GitLab CI
- Primary CI system
- Defined in `.gitlab-ci.yml` and `.gitlab/` directory
- Runs tests, builds, and deployments

### GitHub Actions
- Secondary CI for specific workflows
- Tests about the pull-request settings or repository configuration
- Release automation workflows

## Code Review

Code reviewer plugins for Go and Python are available from the
[Datadog Claude Marketplace](https://github.com/DataDog/claude-marketplace):

- `/go-review`, `/go-improve` - Go code review and iterative improvement
- `/py-review`, `/py-improve` - Python code review and iterative improvement

See the marketplace README for installation instructions.

## Security Considerations

### Sensitive Data
- Never commit API keys or secrets
- Use secret backend for credentials

## Module System
The project uses Go modules with multiple sub-modules.
TODO: Describe specific strategies for managing modules, including any invoke
tasks.

## Platform Support
- **Linux**: Full support (amd64, arm64)
- **Windows**: Full support (Server 2016+, Windows 10+)
- **macOS**: Supported
- **AIX**: No support in this codebase
- **Container**: Docker, Kubernetes, ECS, containerd, and more

## Best Practices
1. **Always run linters before committing**: `dda inv linter.go`
2. **Always test your changes**: `dda inv test --targets=<your_package>`
3. **Follow Go conventions**: Use gofmt, follow project structure
4. **Update documentation**: Keep docs in sync with code changes
6. **Check for security implications**: Review security-sensitive changes carefully

## Troubleshooting Development Issues

### Common Build Issues
- **Missing tools**: Run `dda inv install-tools`
- **CMake errors**: Remove `dda inv rtloader.clean`

### Testing Issues
- **Flaky tests**: Check `flakes.yaml` for known issues
- **Coverage issues**: Use `--coverage` flag
