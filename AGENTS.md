# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` hosts the binaries: `cmd/agent` for the core Agent, `cmd/cluster-agent`, `cmd/dogstatsd`, `cmd/trace-agent`, and eBPF tooling under `cmd/system-probe`.
- Shared Go packages live in `pkg/` (e.g., `pkg/aggregator`, `pkg/collector`, `pkg/config`), while componentized logic is in `comp/` for incremental adoption.
- Python invoke tasks reside in `tasks/`; docs and contributor references are under `docs/` and `docs/dev/`.
- Development configs live in `dev/dist/`; the main runtime config copies to `bin/agent/dist/datadog.yaml` after builds.

## Build, Test, and Development Commands
- `dda inv install-tools` installs the Go, Python, and system tools required for local builds.
- `dda inv agent.build --build-exclude=systemd` produces the primary agent binary without systemd assets; swap in component-specific targets such as `dda inv dogstatsd.build` or `dda inv trace-agent.build` when iterating on those services.
- `dda inv test --targets=./pkg/aggregator` scopes unit tests to a package; omit `--targets` to exercise the full suite.
- `dda inv linter.go` runs `golangci-lint`; prefer `dda inv linter.all` before large merges to surface cross-language issues early.

## Coding Style & Naming Conventions
- Format Go sources with `gofmt` (tabs for indentation, camelCase for identifiers) and rely on `golangci-lint` to enforce project rules.
- Python tooling in `tasks/` follows PEP 8; run `dda inv linter.python` if you touch those scripts.
- Favor descriptive package paths (`pkg/network/`, `comp/core/telemetry`) and snake_case filenames for YAML configs.

## Testing Guidelines
- Go tests use the standard framework; function names must follow `TestXxx`. Table-driven subtests are preferred for coverage clarity.
- Python checks leverage `pytest`; mirror module names with `test_*.py` files.
- Investigate coverage gaps with `dda inv test --targets=<pkg> --coverage` and document notable exclusions in the PR.

## Commit & Pull Request Guidelines
- Recent history shows conventional prefixes (`feat:`, `fix:`, `docs:`) and ticket tags (`[CXP-####]`); follow that pattern and keep subjects under 72 characters.
- Reference issues in the body, outline testing performed, and attach logs or screenshots when UI or observability output changes.
- Pull requests should describe scope, risks, and rollout considerations; note configuration updates so reviewers can flag downstream impacts.

## Security & Configuration Tips
- Never commit secrets; use the secret backend or redacted fixtures for tests.
- Store experimental configuration under `dev/` and guard runtime features with the appropriate Go build tags (see `tasks/build_tags.py`).
- Review changes touching `system-probe` or `security-agent` with dedicated owners—these components ship kernel-space code and warrant extra scrutiny.
