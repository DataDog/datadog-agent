# pkg/shell — Restricted Shell Interpreter

A restricted shell interpreter designed for AI agents performing SRE investigation tasks.
Safety is the primary design goal: the shell defaults to denying all external command execution
and all filesystem access, requiring explicit opt-in via configuration.

For the complete list of supported and blocked shell features, see [SHELL_FEATURES.md](SHELL_FEATURES.md).

## Execution Model

Scripts are processed in two phases:

1. **Parse & Validate** — The script is parsed into an AST, then validated against an allowlist of
   supported syntax nodes.

2. **Execute** — The validated AST is interpreted. Commands are dispatched to builtins or to the
   configured ExecHandler. File access goes through the configured OpenHandler, which enforces
   AllowedPaths restrictions.

## Shell Features

See [SHELL_FEATURES.md](SHELL_FEATURES.md) for the complete list of supported and blocked features.

## Security Model

Every access path is default-deny:

| Resource             | Default                          | Opt-in                                       |
|----------------------|----------------------------------|----------------------------------------------|
| External commands    | Blocked (exit code 127)          | Provide an `ExecHandler`                     |
| Filesystem access    | Blocked                          | Configure `AllowedPaths` with directory list |
| Environment variables| Empty (no host env inherited)    | Pass variables via the `Env` option          |
| Output redirections  | Blocked at validation (exit code 2) | Not configurable — always blocked         |

**AllowedPaths** restricts all file operations (open, read, readdir, exec) to a set of specified
directories. It is built on Go's `os.Root` API, which uses kernel-level `openat` syscalls
for atomic path validation, making it immune to symlink traversal, TOCTOU races, and `..` escape attacks.

## Testing

Tests use a YAML scenario-driven framework located in `tests/scenarios/`.

Scenarios are organized by feature area:

```
tests/scenarios/
├── cmd/          # builtin command tests (echo, cat, exit, true, ...)
└── shell/        # shell feature tests (pipes, variables, control flow, globbing, ...)
```

Good sources of POSIX shell test scenarios:
- [yash POSIX shell test suite](https://github.com/magicant/yash/tree/trunk/tests)

## Platform Support

Linux, macOS, and Windows.

## Tips

Prompt for generate test scenarios:
```
Improve pkg/shell/tests scenarios coverage by taking inspiration from pkg/shell/yash_posix_tests

Notes:
- avoid duplicate test coverage, if you encounter duplicate scenarios, remove or merge them
- create as many new scenarios as possible (no limit), of course they must be valuable
- if some tests fail, keep in mind it's possible that pkg/shell implementation is wrong, and it's fine to fix the implementation
```
