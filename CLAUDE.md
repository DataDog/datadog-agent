# Datadog Agent - Project Overview for Claude

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

### Prerequisites
1. Go 1.24+ with `$GOPATH` set and `$GOPATH/bin` in PATH
2. Python 3.12 with `pip install dda`
3. CMake 3.15+ and C++ compiler

### Common Commands

#### Building
```bash
# Install development tools
uvx dda inv install-tools

# Build the main agent
uvx dda inv agent.build --build-exclude=systemd

# Build specific components
uvx dda inv dogstatsd.build
uvx dda inv trace-agent.build
uvx dda inv system-probe.build
```

#### Testing
```bash
# Run all tests
uvx dda inv test

# Test specific package
uvx dda inv test --targets=./pkg/aggregator

# Run Go linters
uvx dda inv linter.go

# Run all linters
uvx dda inv linter.all
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
- Run with `uvx dda inv test --targets=<package>`

### Integration Tests
- Located in `test/integration/`
- Run with `uvx dda inv integration-tests`

### End-to-End Tests
- New E2E framework in `test/new-e2e/`

### Linting
- Go: golangci-lint via `uvx dda inv linter.go`
- Python: various linters via `uvx dda inv linter.python`
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
Go build tags control feature inclusion:
- `kubeapiserver` - Kubernetes API server support
- `containerd` - containerd support
- `docker` - Docker support
- `ebpf` - eBPF support
- `python` - Python check support

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
- Windows unit tests
- Code coverage reporting

## Security Considerations

### Sensitive Data
- Never commit API keys or secrets
- Use secret backend for credentials
- Scrub sensitive data from logs

### Code Paths
- `/pkg/security/` - Security monitoring code
- `/pkg/compliance/` - Compliance checking
- eBPF code is GPL-licensed (separate from Apache 2.0)

## Debugging Tips

1. **Enable debug logging**: Set `log_level: debug` in config
2. **Check agent status**: `./bin/agent/agent status`
3. **Inspect flare**: `./bin/agent/agent flare`
4. **View check output**: `./bin/agent/agent check <check_name>`

## Module System
The project uses Go modules with multiple sub-modules. Key modules:
- Main module: `github.com/DataDog/datadog-agent`
- Sub-modules defined in `modules.yml`
- Use `uvx dda inv modules.show` to list all modules

## Platform Support
- **Linux**: Full support (amd64, arm64)
- **Windows**: Full support (Server 2016+, Windows 10+)
- **macOS**: Development and testing
- **AIX**: No support in this codebase
- **Container**: Docker, Kubernetes, ECS, containerd, and more

## Best Practices

1. **Always run linters before committing**: `uvx dda inv linter.go`
2. **Test your changes**: `uvx dda inv test --targets=<your_package>`
3. **Follow Go conventions**: Use gofmt, follow project structure
4. **Update documentation**: Keep docs in sync with code changes
5. **Use meaningful commit messages**: Follow conventional commits
6. **Check for security implications**: Review security-sensitive changes carefully

## Troubleshooting Development Issues

### Common Build Issues
- **Missing tools**: Run `uvx dda inv install-tools`
- **CMake errors**: Remove `uvx dda inv rtloader.clean`

### Testing Issues
- **Flaky tests**: Check `flakes.yaml` for known issues
- **Integration test failures**: Ensure services are running
- **Coverage issues**: Use `--coverage` flag

## Additional Resources
- [Developer Documentation](docs/dev/README.md)
- [Agent User Documentation](https://docs.datadoghq.com/agent/)
- [Contributing Guidelines](CONTRIBUTING.md)
- [Public Datadog Agent Docs](https://docs.datadoghq.com/developers/dogstatsd/)