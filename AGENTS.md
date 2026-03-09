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

### Contributing
PRs should follow `.github/PULL_REQUEST_TEMPLATE.md` and the guidelines in
`docs/public/guidelines/` (contributing, coding style, components, etc.). When
a PR changes behavior, configuration options, or APIs, update the corresponding
documentation in the same PR — not as a follow-up.

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

## Review guidelines

The following are areas of particular concern for this codebase. They highlight
project-specific risks that have led to production bugs in the Datadog Agent.

### E2E coverage with fakeintake
The E2E framework (`test/new-e2e/`) uses **fakeintake**, a mock Datadog intake
that captures metrics, logs, traces, and check runs. When a change affects
user-visible behavior (new metrics, changed log output, modified payloads),
check whether an E2E test asserts the expected data arrives in fakeintake. Unit
tests alone are not sufficient for validating the agent's end-to-end data
pipeline.

### Branch-conditional CI creates blind spots
Most E2E tests only run on `main`, release branches (`N.N.x`), and RC tags —
**not on PR branches**. This means some classes of bugs cannot be caught before
merge. Be extra careful reviewing:
- Packaging or installation changes (MSI, deb, rpm, BUILD.bazel)
- Agent startup/shutdown sequences
- Cross-component communication (e.g. system-probe ↔ agent)

These changes are likely to need `qa/rc-required`.

### Multi-platform divergence
The agent ships on Linux, Windows, and macOS. Platform-specific code paths (via
`runtime.GOOS`, build tags, OS-specific file paths) are a frequent source of
bugs — typically the "other" platform is untested. The same applies to
packaging: Windows MSI and Linux deb/rpm have independent logic that can
silently diverge.

### Concurrency and component lifecycle
The agent runs many concurrent goroutines with explicit `Start()`/`Stop()`
lifecycles. The most common bugs are send-on-closed-channel during shutdown and
goroutine leaks. Changes that introduce goroutines or modify component lifecycle
should have tests exercising startup and graceful shutdown.

### Graceful degradation during startup
Components initialize in stages — some dependencies may not be ready when others
start. Functions exposed to UIs or APIs should return safe defaults when a
dependency is unavailable, not propagate errors or panic.

### Stale documentation
If a PR changes behavior but doesn't update the corresponding docs, comments,
or doc strings, flag it. Stale docs lead to bugs: contributors build on
incorrect assumptions.

## Keeping AI context accurate

AI agents read `AGENTS.md`, `CLAUDE.md`, and skill files to understand the
codebase. These files must stay accurate — stale guidance causes recurring
mistakes across sessions.

### File hierarchy

```
AGENTS.md                          ← repo-wide: architecture, workflow, review guidelines
├── test/e2e-framework/AGENTS.md   ← E2E framework: environments, provisioners, agentparams
├── test/fakeintake/AGENTS.md      ← fakeintake: endpoints, client API, extension guide
├── pkg/.../AGENTS.md              ← package-level: structure, patterns, pitfalls
└── .claude/skills/*/SKILL.md      ← task-specific: step-by-step procedures
```

Each level inherits context from its parent via `CLAUDE.md` (`@../../CLAUDE.md`
→ `@AGENTS.md`). Keep information at the right level — don't duplicate
repo-wide rules in sub-project files.

### What to update and when

| File | Update when |
|------|-------------|
| `AGENTS.md` (root) | Architecture, workflow, build commands, or review guidelines change |
| Sub-project `AGENTS.md` | APIs, conventions, or extension patterns in that sub-project change |
| `.claude/skills/*/SKILL.md` | A skill's steps, examples, or recommendations become outdated |

Keep rules generalizable. A good guideline covers a class of bugs, not a single
incident. Think bias/variance: too specific and it only catches one bug; too
generic and it's noise.

### Self-improvement expectation

**AI agents:** when working on any task (reviewing, writing code, running
tests), if you notice a gap or inaccuracy in an `AGENTS.md` or skill file, fix
it — either in the same PR or as a follow-up. Small, incremental improvements
are preferred over large rewrites. This creates a feedback loop where every
session leaves the context more accurate for the next one.
