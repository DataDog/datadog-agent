# Datadog Agent - Project Overview for AI coding assistant

## Project Summary
The Datadog Agent collects metrics, traces, logs, and security events and forwards them to the Datadog platform. Written primarily in Go; this is the main repository for Agent versions 6 and 7.

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

- `/comp/` - Component-based architecture modules (Fx components)

- `/tasks/` - Python invoke tasks for development
  - Build, test, lint, and deployment automation

- `/rtloader/` - Runtime loader for Python checks

- `/packages/` - The declarations of what goes into each package we build for distribution.

- `/omnibus/` - The legacy build system. Still in use, but we are trying not to add to it.


## Development Workflow

### Critical: Always use `dda inv`, never raw `go` commands

This project uses extensive custom Go build tags. Most source files are ignored
by the standard Go toolchain unless the correct tags are passed. The `dda inv`
wrapper tasks (defined in `tasks/`) compute the right build tags automatically.

**Never run these commands directly:**

| Instead of | Use |
|---|---|
| `go build …` | `dda inv agent.build`, `dda inv cluster-agent.build`, etc. |
| `go test …` | `dda inv test --targets=./pkg/…` |
| `go mod tidy` | `dda inv tidy` |
| `go vet …` | `dda inv linter.go` |
| `golangci-lint run …` | `dda inv linter.go` |

This also applies to indirect usage — do not shell out to `go build` or
`go test` for compilation checks. If you need to verify that code compiles,
build the relevant component with `dda inv *.build`.

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

# Build specific packages
bazel build //packages/agent/linux:debian

# Keep BUILD.bazel files in sync with go dependencies
dda inv tidy
```

#### Testing
```bash
# Run all tests
dda inv test

# Test specific package
dda inv test --targets=./pkg/aggregator
bazel test //pkg/aggregator/...

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

### eBPF Bazel Build
eBPF programs, runtime-compilation bundles, and cgo godefs are built with Bazel — see `bazel/AGENTS.md` (§ eBPF programs and code generation).

## Testing Strategy

### Unit Tests
Go tests run via `dda inv test --targets=<package>` (see the `dda inv` table above) or `bazel test //pkg/... //comp/...`; Python checks use pytest.

### End-to-End Tests
- E2E tests live in `test/new-e2e/tests/` and use the framework in `test/e2e-framework/`
- Tests provision real AWS, GCP or Azure infrastructure, deploy the agent, and assert payloads
  arrive in **fakeintake**. By default it forwards payloads to `dddev` org account.
- Key docs: `test/e2e-framework/AGENTS.md` (framework), `test/fakeintake/AGENTS.md`
  (intake mock), `docs/public/how-to/test/e2e.md` (setup & running)
- Use `/write-e2e` skill or read those docs directly to write new E2E tests
- Run locally: `dda inv new-e2e-tests.run --targets=./tests/<area>/...`

### Linting
- Python: various linters via `dda inv linter.python`
- YAML: yamllint
- Shell: shellcheck

## Build System

### Invoke Tasks
The project uses Python's Invoke framework for custom tasks. Run `dda inv -l` to list them.

### Build Tags
Go build tags control feature inclusion, some examples are:
- `kubeapiserver` - Kubernetes API server support
- `containerd` - containerd support
- `docker` - Docker support
- `ebpf` - eBPF support
- `python` - Python check support
- and MANY more, refer to ./tasks/build_tags.py for a full reference.

## Important Files
- `datadog.yaml` - Main agent configuration
- `modules.yml` - Go module definitions
- `release.json` - Release version information
- `.gitlab-ci.yml` - CI/CD pipeline configuration

## CI/CD Pipeline

### GitLab CI
- Primary CI system
- Defined in `.gitlab-ci.yml` and `.gitlab/` directory
- Runs tests, builds, and deployments

#### Fetching CI job logs locally

`gitlab.ddbuild.io` is OAuth-gated, so `curl` can't fetch traces. Use
`dda inv gitlab.print-job-trace <job_id>` (`dda` handles auth). The `<job_id>`
is the trailing path of a `.../builds/<job_id>` URL, found in the `dd-gitlab/*`
rows of `gh pr checks <pr_number>`. See `dda inv -l | grep gitlab` for more.

### GitHub Actions
Secondary CI: pull-request/repository-configuration checks and release automation.

### Contributing
PRs should follow `.github/PULL_REQUEST_TEMPLATE.md` and the guidelines in
`docs/public/guidelines/` (contributing, coding style, components, etc.).

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

## Platform Support
Ships on Linux (amd64, arm64), Windows (amd64), and macOS (amd64, arm64), plus containers (Docker/Kubernetes/ECS/containerd). AIX is not yet supported but in progress.

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
├── bazel/AGENTS.md                ← Bazel build system: conventions, pitfalls, rule writing
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
