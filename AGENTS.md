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
# Install development tools
dda inv install-tools

# Build the main agent
dda inv agent.build --build-exclude=systemd

# Build specific components
dda inv dogstatsd.build
dda inv trace-agent.build
dda inv system-probe.build
```

#### Testing
```bash
# Run all tests
dda inv test

# Test specific package
dda inv test --targets=./pkg/aggregator

# Run Go linters
dda inv linter.go

# Run all linters
dda inv linter.all
```

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

