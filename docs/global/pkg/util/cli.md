# pkg/util/cli

## Purpose

Provides helpers for migrating CLI flag syntax as the agent's command structure evolves. Specifically, it handles the replacement of non-POSIX single-dash flags (`-config`) with POSIX double-dash equivalents (`--config`) and the substitution of deprecated flags with their modern sub-command equivalents, emitting a deprecation warning to an `io.Writer` on each replacement.

## Key elements

### Types

| Type | Description |
|---|---|
| `ReplaceFlag` | Holds the replacement `Args []string` (may be more than one token, e.g. when splitting `--foo=bar` into `--foo bar`) and a `Hint string` used in the warning message. |
| `ReplaceFlagFunc` | `func(arg, flag string) ReplaceFlag` — a function that produces the replacement given the original argument and the matched flag prefix. |

### Functions

| Function | Description |
|---|---|
| `ReplaceFlagPosix(arg, flag string) ReplaceFlag` | Prepends an extra `-` to the matched flag, turning `-foo` into `--foo`. |
| `ReplaceFlagExact(replaceWith string) ReplaceFlagFunc` | Returns a `ReplaceFlagFunc` that substitutes the matched flag with a literal string. |
| `ReplaceFlagSubCommandPosArg(replaceWith string) ReplaceFlagFunc` | Returns a `ReplaceFlagFunc` that splits `--foo=bar` into two tokens `[replaceWith, "bar"]`, useful when a flag is being converted into a positional argument of a sub-command. |
| `FixDeprecatedFlags(args []string, w io.Writer, m map[string]ReplaceFlagFunc) []string` | Iterates over `args`, applies the first matching replacement from `m` for each argument, writes a deprecation warning to `w`, and returns the rewritten argument list. Unmatched arguments are passed through unchanged. |

## Usage

Used in process-agent and trace-agent deprecated flag shims to transparently rewrite legacy invocations while warning the operator:

```go
// cmd/trace-agent/command/deprecated.go
// cmd/process-agent/command/deprecated.go

import "github.com/DataDog/datadog-agent/pkg/util/cli"

args = cli.FixDeprecatedFlags(os.Args[1:], os.Stderr, map[string]cli.ReplaceFlagFunc{
    "-config": cli.ReplaceFlagPosix,
    "--pid":   cli.ReplaceFlagExact("--pid-file"),
})
```

A deprecation warning in the form `WARNING: '-config' argument is deprecated and will be removed in a future version. Please use '--config' instead.` is printed for each replaced flag.

## Notes

- The matching is prefix-based (`strings.HasPrefix`), so `--foo=bar` matches a key of `--foo`.
- Only the first matching rule is applied per argument (early exit on match).
- No build constraints; works on all platforms.
