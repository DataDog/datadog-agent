# pkg/shell — Safe Shell Interpreter

## Overview

This is a minimal sh/POSIX compatible shell interpreter.
Safety is the primary goal.

This shell is intended to be used by AI Agents.

## Platform Support

The shell is supported on Linux, Windows and macOS.

## Documentation

- `README.md` and `SHELL_FEATURES.md` must be kept up to date with the implementation.

## Testing

- In test scenarios, use `expect.stderr` when possible instead of `stderr_contains`.
- `test_against_local_shell` should be enabled (the default) when the tested feature is sh/POSIX compliant. Only set `test_against_local_shell: false` for features that intentionally diverge from standard sh behavior (e.g. blocked commands, restricted redirects, readonly enforcement).
