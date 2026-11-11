# Contributing to rshell

First of all, thanks for contributing!

## Submitting Issues

Use the GitHub issue tracker to report bugs or request features. Before opening a new issue, search existing issues to avoid duplicates.

## Setup

From the Agent repository root:

```sh
bazel run //pkg/rshell/cmd/rshell -- --allow-all-commands -c 'help'
dda inv test --module=pkg/rshell --race
```

The Makefile in `pkg/rshell` wraps the same Agent build and test tooling.

## Core Principle: Match Bash Behavior

This shell must behave like bash/POSIX shells. If the shell's output differs from bash, **fix the shell** — never "fix" a test by changing its expected output to match broken behavior. Only diverge intentionally (e.g. sandbox restrictions, blocked commands).

## Adding a Builtin

See `docs/RULES.md` for the underlying implementation rules.

## Testing

- `make test` — Go unit tests.
- `make test_against_bash` — runs `tests/scenarios/*.yaml` against real bash in Docker and diffs output byte-for-byte. Required before submitting any change touching `tests/scenarios/` or a builtin.
- Prefer adding a scenario test (`tests/scenarios/`) over a Go test; only use Go tests when a scenario cannot express the required behavior.

When adding a Go package, include its `source_files` filegroup in
`pkg/rshell/BUILD.bazel` so the source-scanning tests see it. CI checks that every
Go source file outside `testdata` is included.

The Linux selective-elevation seccomp test and both Landlock enforcement tests
require `CGO_ENABLED=0`, matching the production privileged helper. The other
sandbox tests and the external-helper integration driver also run in cgo/race
builds. CI has a dedicated root job that requires all four enforcement and
integration checks to pass; skipped checks fail that job. The helper binary must
be executable and its parent directories traversable by the service account
(`nobody` in the integration test).

To exercise these checks locally on a Linux host with Landlock ABI 3+ and seccomp,
from `pkg/rshell`:

```bash
CGO_ENABLED=0 go test -c -o /tmp/rshell-landlock.test ./internal/sandbox/landlock
CGO_ENABLED=0 go test -c -o /tmp/rshell-seccomp.test ./internal/sandbox/seccomp
CGO_ENABLED=0 go build -o /tmp/rshell-root-helper ./cmd/rshell
go test -race -c -o /tmp/rshell-root-integration.test ./cmd/rshell
sudo /tmp/rshell-landlock.test -test.v
sudo /tmp/rshell-seccomp.test -test.v
sudo env RSHELL_ROOT_INTEGRATION=1 RSHELL_BINARY=/tmp/rshell-root-helper \
  /tmp/rshell-root-integration.test -test.v -test.run='^TestPrivilegedHelperRootIntegration$'
```

## Code Review

Follow the Agent repository's code review guidelines.

If your change touches `analysis/` (the import allowlists), it must be reviewed by a human, who will add the `verified/analysis` label — this cannot be automated.

## Pull Requests

To submit your changes:

1. Fork the repository.
2. Create a topic branch from `main`.
3. Make your changes — add tests for any new functionality.
4. Run `make fmt` and `make test` to ensure the code is formatted and all tests pass.
5. Push your branch and open a pull request against `main`.

Keep your pull requests focused on a single change to make review easier.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Write tests for new functionality and bug fixes.
- Keep changes minimal and focused.

## License

By contributing to this project you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
