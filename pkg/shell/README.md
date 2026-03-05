# pkg/shell — Restricted Shell Interpreter

A restricted shell interpreter designed for AI agents performing SRE investigation tasks.
Safety is the primary design goal: the shell defaults to denying all external command execution
and all filesystem access, requiring explicit opt-in via configuration.

## Execution Model

Scripts are processed in two phases:

1. **Parse & Validate** — The script is parsed into an AST, then validated against an allowlist of
   supported syntax nodes. Disallowed constructs (e.g., `if`, `while`, command substitution) are
   rejected with exit code 2 _before any code runs_. This guarantees that partially executed
   scripts cannot occur due to an unsupported feature mid-script.

2. **Execute** — The validated AST is interpreted. Commands are dispatched to builtins or to the
   configured ExecHandler. File access goes through the configured OpenHandler, which enforces
   AllowedPaths restrictions.

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
directories. It uses kernel-level `openat` syscalls for atomic path validation, making it immune
to symlink traversal, TOCTOU races, and `..` escape attacks.

## Configuration

The interpreter is configured through `RunnerOption` functions passed to `New()`:

- **`StdIO(in, out, err)`** — Sets stdin, stdout, stderr streams.
- **`AllowedPaths(dirs)`** — Restricts filesystem access to the given directories.
- **`Env`** — Sets the initial environment variables (empty by default).
- **`ExecHandler`** — Handles external command execution. Defaults to `NoExecHandler` which rejects all commands.

## Testing

Tests use a YAML scenario-driven framework located in `tests/scenarios/`. Each `.yaml` file defines
a self-contained test with `input.script`, optional `setup.files`, and `expect` (stdout, stderr, exit_code).

Scenarios are organized by feature area:

```
tests/scenarios/
├── cmd/          # builtin command tests (echo, cat, exit, true, false)
└── shell/        # shell feature tests (pipes, variables, control flow, globbing, ...)
```

## Platform Support

Linux, macOS, and Windows.
