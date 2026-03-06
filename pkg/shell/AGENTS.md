# pkg/shell — Safe Shell Interpreter

## Overview

This is a minimal bash/POSIX like shell interpreter.
Safety is the primary goal.

This shell is intended to be used by AI Agents.

## Platform Support

The shell is supported on Linux, Windows and macOS.

## Documentation

- `README.md` and `SHELL_FEATURES.md` must be kept up to date with the implementation.

## Testing

- In test scenarios, use `expect.stderr` when possible instead of `stderr_contains`.
