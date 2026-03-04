---
name: implement-posix-command
description: Implement a new POSIX command as a builtin in the safe shell interpreter
argument-hint: "<command-name>"
---

Implement the **$ARGUMENTS** command as a builtin in `pkg/shell/interp/`.

## Context

The safe shell interpreter (`pkg/shell/interp/`) implements all commands as Go builtins — it never executes host binaries. All security and safety constraints are defined in `.claude/skills/implement-posix-command/RULES.md`. Read that file first before writing any code.

## Step 1: Research the command

Before writing any code:

1. Read `.claude/skills/implement-posix-command/RULES.md` in full.
2. Read the POSIX specification behavior for **$ARGUMENTS** — what flags are standard, what flags are dangerous (write/execute), and what the expected output format is.

## Step 2: User confirms which flags to implement

Based on your research, suggest which flags should originally be supported as part of implementing this command.
All flags must obey the rules from RULES.md. Our goal here is to implement the most common flags which
obey RULES.md. Use your knowledge of these tools to help determine which flags are common and worth implementing.
For the original implementation, err on the side of selecting fewer, more important flags.

Determine:
- Which flags are safe to support (read-only, no exec)
- Which flags MUST be rejected with a clear error (any that write, delete, or execute)
- stdin support (does the command read from stdin when no files are given?)
- Exit code behavior (when should it return 0 vs 1?)
- Memory safety approach (streaming vs buffered, max sizes)
- Whether the command could read indefinitely from an infinite source (e.g. stdin from /dev/zero) — if so, it will need `context.Context` threading (see Step 5)

Show the user a summary that describes each standard flag
you found in the POSIX documentation. Group the flags by "will implement", "maybe implement", and "do not implement."
For each flag, show the flag name and a very brief (1-2 sentence) description of what it does.

Enter plan mode with `EnterPlanMode` and present the flag list and implementation approach. Wait for user approval.

Once the user has confirmed the flags to be implemented, we will create the first bit of Go code
for our command implementation. Within `datadog-agent/pkg/shell/interp`, create "builtin_$ARGUMENTS.go"
which will contain our implementation. For now, just write the package header and a detailed docstring
which describe the command and lists out all of the accepted flags that will be implemented.

## Step 3: Set up POSIX tests

The GNU source is available at https://ftp.gnu.org/gnu/coreutils/. Go there, find the latest stable
release, and download and extract it into /tmp somewhere.

We want to extract some script-based tests for the relevant command. Within the coreutils source,
look in `tests/$ARGUMENTS`. If that folder exists and has a bunch of .sh files in it, those are our
tests. We want to copy these into our repository at `datadog-agent/pkg/shell/interp/builtintest/$ARGUMENTS/gnu_scripts`.

Once they are copied, we now want to go through each test file and do the following (parallelize this per test file):

1. Determine the purpose of the test. If the test is wholly concerned with a flag we have decided not to implement, delete the test. Otherwise, we will keep the test and continue this checklist.
2. Modify the `init.sh` sourcing line. The path must be captured as an **absolute path before `setup_` runs**, because `setup_` changes the working directory to a temp dir and relative paths will break. Use this pattern at the top of every script:
   ```sh
   srcdir=$(cd "$(dirname "$0")/../.." && pwd)
   . "$srcdir/init.sh"
   ```
3. Modify the test to remove any extraneous usages of flags that we are not implementing.
4. Modify the test to invoke $ARGUMENTS via the agent binary. Declare a helper at the top of the script and use it for every invocation:
   ```sh
   AGENT_BIN=${AGENT_BIN:-}
   if test -z "$AGENT_BIN"; then
     AGENT_BIN=$(command -v agent 2>/dev/null || true)
   fi
   if test -z "$AGENT_BIN" || ! test -x "$AGENT_BIN"; then
     skip_ "agent binary not found; build with: dda inv agent.build"
   fi

   safe_shell() { "$AGENT_BIN" shell --command "$*"; }
   ```
   Replace every bare invocation of `$ARGUMENTS` with `safe_shell "$ARGUMENTS ..."`.

After adapting the scripts, verify they pass by running them manually with a built agent binary.

## Step 4: Implement Go tests

Go tests live alongside the other builtins in the `pkg/shell/interp/` package, **not** in a subdirectory:

- **Go tests** → `pkg/shell/interp/builtin_$ARGUMENTS_test.go` (`package interp`)
- **Shell scripts** → `pkg/shell/interp/builtintest/$ARGUMENTS/gnu_scripts/` (already done in Step 3)

Using `package interp` lets the tests access unexported helpers and constants (e.g. for clamping checks).

Tests should be written to the following specifications:

- All implemented flags are exercised in at least one test
- Review RULES.md and write tests verifying that the rules are honored where possible, checking for runaway memory allocations, infinite loops / hangs, etc
- Use `os.DevNull` instead of hardcoded `/dev/null` so tests compile on all platforms
- For tests that are inherently platform-specific (symlinks, Windows reserved names, directory reads), create separate files with build tags:
  - `builtin_$ARGUMENTS_unix_test.go` with `//go:build unix` at the top
  - `builtin_$ARGUMENTS_windows_test.go` with `//go:build windows` at the top
- When writing tests that pipe through another builtin (e.g. `cat file | $ARGUMENTS`), account for that builtin's output behaviour. For example, the `cat` builtin uses `fmt.Fprintln` which adds a trailing `\n` to each line — a binary file piped through `cat` will have a `\n` appended that was not in the original file.

Verify the tests build and all fail (since we have no implementation yet).

## Step 5: Implement the $ARGUMENTS command

Create `pkg/shell/interp/builtin_$ARGUMENTS.go` following the patterns in the existing builtins:

1. **Function signature**: Use `func (r *Runner) builtin$ARGUMENTS(args []string) error` for commands that read bounded input. If the command reads from potentially infinite sources (stdin, pipes, /dev/zero) and needs to respect the executor timeout, use `func (r *Runner) builtin$ARGUMENTS(ctx context.Context, args []string) error` instead and check `ctx.Err()` before every read in any loop.
2. Parse flags with pflag; any flag not explicitly registered is automatically rejected by pflag — do NOT add pre-scan loops or special-case flag rejection; also register `-h`/`--help` and handle it per RULES.md
3. Implement the command body with bounded reads and proper error handling
4. Set `r.exitCode` appropriately
5. Write errors to `r.stderr`, output to `r.stdout`

Register the command in `pkg/shell/interp/builtin.go`:
- Add it to the `builtins` map
- Add a `case` in the `builtin()` switch statement
- If your function takes `ctx context.Context`, make sure the `builtin()` function passes `ctx` (not `_`) — update the parameter name in the `builtin()` signature if needed

**Do not modify any existing files** (e.g. `interp_test.go`) unless directly required by the registration step above.

## Step 6: Verify and Harden

Run the tests:

```bash
dda inv test --targets=./pkg/shell/interp
```

Fix any failures before finishing.

After the initial test suite is passing, write another round of tests focused on:

- 100% code coverage of the implementation
- Additional tests specific to the rules in RULES.md. For example, if the implementation passes user input into buffer allocations, ensure in tests that this input is clamped to an appropriate value and not passed as-is to the buffer.

## Step 7: Code review

Run two review passes in parallel, then fix every finding before finishing.

### Part A: RULES.md compliance

Spawn parallel review agents — one per section of RULES.md — to audit the final implementation and test suite against every rule:

- Memory Safety & Resource Limits + DoS Prevention + Special File Handling
- Input Validation & Error Handling + Integer Safety
- Cross-Platform Compatibility + Output Consistency
- Testing Requirements (verify every rule has corresponding test coverage)

### Part B: General Go code quality

Review the implementation for standard Go best practices:

- **Error handling**: every `io.Writer.Write`, `io.Copy`, and `fmt.Fprintf` to a writer must have its error checked or explicitly discarded with `_`
- **Context cancellation**: `ctx.Err()` must be checked at the top of every loop that reads input — including scanner loops, not just explicit `Read` calls
- **Resource cleanup**: `defer` must be used to close files and other resources; when a file is opened inside a loop, use an IIFE (`func() error { defer f.Close(); ... }()`) to scope the defer to the loop iteration rather than the function
- **DRY**: functions that differ only in variable names or error strings must be merged; use a `kind string` parameter for error messages
- **Sentinel values**: `-1` or other magic sentinel ints used to select between modes should be replaced by a named `type … int` with named constants
- **Redundant conditionals**: simplify boolean expressions to the minimum necessary branches (e.g. `(a || b) && !c` instead of `(a && !c) || b` followed by `if c { … = false }`)
- **Variable re-derivation**: the same logical value must not be encoded twice in different types (e.g. both a `byte` and a `string` for the line separator)
- **Test helpers**: a test must not run the same command twice just to observe different aspects; consolidate into a single runner that captures both stdout/stderr and exit code

For each issue found in either review, fix it immediately. Re-run tests after all fixes. Do not declare the implementation done until every finding is resolved.
