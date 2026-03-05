---
name: implement-posix-command
description: Implement a new POSIX command as a builtin in the safe shell interpreter
argument-hint: "<command-name>"
---

Implement the **$ARGUMENTS** command as a builtin in `pkg/shell/interp/`.

---

## ⛔ STOP — READ THIS BEFORE DOING ANYTHING ELSE ⛔

You MUST follow this execution protocol. Skipping steps has caused defects in every prior run of this skill.

### 1. Create the full task list FIRST

Your very first action — before reading ANY files, before writing ANY code — is to call TaskCreate exactly 8 times, once for each step below (Steps 1–8). Use these exact subjects:

1. "Step 1: Research the command"
2. "Step 2: User confirms which flags to implement"
3. "Step 3: Set up POSIX tests"
4. "Step 4: Implement Go tests"
5. "Step 5: Implement the $ARGUMENTS command"
6. "Step 6: Verify and Harden"
7. "Step 7: Code review"
8. "Step 8: Exploratory pentest"

### 2. Execution order and gating

Steps run in this order:

```
Step 1 → Step 2 → Steps 3 + 4 + 5 (parallel) → Step 6 → Step 7 → Step 8
```

**Sequential steps (1 → 2):** Before starting step N, call TaskList and verify step N-1 is `completed`. Set step N to `in_progress`.

**Parallel steps (3, 4, 5):** Once Step 2 is `completed`, set Steps 3, 4, and 5 all to `in_progress` at the same time and work on all three concurrently. The implementation (Step 5) and the tests (Steps 3, 4) are all guided by the approved spec from Step 2 — they do not need to wait for each other.

**Convergence (6 → 7 → 8):** Before starting Step 6, call TaskList and verify Steps 3, 4, AND 5 are all `completed`. Then proceed sequentially through 6 → 7 → 8.

Before marking any step as `completed`:
- Re-read the step description and verify every sub-bullet is satisfied
- If any sub-bullet is not done, keep working — do NOT mark it completed

### 3. Never skip steps

- Do NOT skip research (Step 1) because you think you already know the command
- Do NOT skip shell tests (Step 3) — download and adapt the GNU coreutils tests
- Do NOT skip review (Step 7) or pentest (Step 8) because "tests pass"
- Steps 1 and 2 require user interaction — do NOT auto-approve on the user's behalf

If you catch yourself wanting to skip a step, STOP and do the step anyway.

---

## Context

The safe shell interpreter (`pkg/shell/interp/`) implements all commands as Go builtins — it never executes host binaries. All security and safety constraints are defined in `.claude/skills/implement-posix-command/RULES.md`. Read that file first before writing any code.

Key structural facts about this codebase:
- Builtin implementations live in `pkg/shell/interp/builtins/` (`package builtins`), one file per command
- Each builtin is a standalone function (not a method on Runner): `func builtinCmd(ctx context.Context, callCtx *CallContext, args []string) Result`
- File access MUST go through `callCtx.OpenFile()` — never `os.Open()` directly
- Output goes to `callCtx.Stdout`/`callCtx.Stderr` via `callCtx.Out()`, `callCtx.Outf()`, `callCtx.Errf()`
- Return `Result{}` for success, `Result{Code: 1}` for failure
- Builtins are registered in the `registry` map in `pkg/shell/interp/builtins/builtins.go`

## Step 1: Research the command

Before writing any code:

1. Read `.claude/skills/implement-posix-command/RULES.md` in full.
2. Read the POSIX specification behavior for **$ARGUMENTS** — what flags are standard, what flags are dangerous (write/execute), and what the expected output format is.
3. Read the associated GTFOBins recommendations, if any, which can be found at https://gtfobins.org/gtfobins/$ARGUMENTS. These contain information on unsafe flags and vulnerabilities that we will need to avoid.

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
for our command implementation. Create `pkg/shell/interp/builtins/$ARGUMENTS.go` (`package builtins`)
with just the package header and a detailed doc comment describing the command and listing all
accepted flags that will be implemented.

## Step 3: Set up POSIX tests

**GATE CHECK**: Call TaskList. Step 2 must be `completed` before starting this step. Set Steps 3, 4, and 5 all to `in_progress` now — they run in parallel.

Download the GNU coreutils source for reference:

```bash
# GitHub mirror is more reliable than ftp.gnu.org
curl -sL https://github.com/coreutils/coreutils/archive/refs/heads/master.tar.gz | tar -xz -C /tmp
```

Look in `/tmp/coreutils-master/tests/$ARGUMENTS/` for the GNU test cases. For each test file:

1. **Filter**: Skip tests wholly concerned with flags we decided not to implement (e.g. `--follow`, inotify, `--pid`). Also skip tests that rely on obsolete POSIX2 syntax (e.g. `_POSIX2_VERSION` env var, combined flag+number forms like `-1l`), platform-specific kernel features (`/proc`, `/sys`), or the GNU test framework helpers (`retry_delay_`, `compare`, `framework_failure_`).

2. **Translate**: For each remaining test case, create one YAML scenario file at `pkg/shell/tests/scenarios/cmd/$ARGUMENTS/`. The YAML format is:

```yaml
description: One sentence describing what this scenario tests.
setup:
  files:
    - path: relative/path/in/tempdir
      content: "file content here"
      chmod: 0644           # optional
      symlink: target/path  # optional; creates a symlink instead of a file
input:
  allowed_paths: ["$DIR"]   # "$DIR" resolves to the temp dir; omit to block all file access
  script: |+
    $ARGUMENTS some/file
expect:
  stdout: "expected output\n"    # exact match
  stdout_contains: ["substring"] # list; use instead of stdout for partial matches
  stderr: ""                     # exact match; use stderr_contains for partial matches
  stderr_contains: ["partial"]   # list
  exit_code: 0
```

**`stdout_contains` and `stderr_contains` must be YAML lists**, not scalar strings.
`stdout_contains: "text"` is invalid — always write `stdout_contains: ["text"]`.

Group scenario files into subdirectories by concern (e.g. `lines/`, `bytes/`, `headers/`, `stdin/`, `errors/`, `hardening/`).

**`stderr` vs `stderr_contains`**: Prefer `expect.stderr` (exact match) over `stderr_contains` (substring) unless the error message contains platform-specific text.

Note the source test in a comment at the top of each YAML file (e.g. `# Derived from GNU coreutils tail.pl test n-3`).

Write scenarios covering:
- Each implemented flag at least once
- Edge cases: empty file, single-line file, file with no trailing newline
- Error cases: missing file, directory as argument, invalid flag/argument values
- Flags that should be rejected (e.g. `-f`, `--follow`): verify `exit_code: 1` and stderr message

## Step 4: Implement Go tests

**PARALLEL STEP**: This runs concurrently with Steps 3 and 5. No gate check needed — Step 2 being `completed` is sufficient.

Go tests live alongside the other builtins in the `pkg/shell/interp/` package, **not** in a subdirectory:

- **Go tests** → `pkg/shell/interp/builtin_$ARGUMENTS_test.go` (`package interp`)
- **Shell scripts** → `pkg/shell/interp/test/shell/$ARGUMENTS/` (already done in Step 3)

The shell scripts are run automatically by `go test ./pkg/shell/interp` via
`pkg/shell/interp/shell_scripts_test.go`, which discovers every `test/shell/*/*.sh`
and runs it with `sh` (skipping gracefully if `sh` or the agent binary is absent).
No extra CI configuration is required.

All test files use `package interp_test` (the external test package). Do **not** use `package interp`. Unexported helpers do not need to be tested directly — test them through their exported surface.

### Exit code behaviour in Go tests

`runScript` returns `(stdout, stderr string, exitCode int)` — you can assert the exit code directly
without writing any custom helper. Builtins signal failure via `Result{Code: 1}`, which the
interpreter converts to an `ExitStatus` error that `runScript` already unwraps for you.

To verify that a command rejected a bad flag or argument, check both stderr and the returned exit
code:

```go
_, stderr, code := runScript(t, "tail --follow file", dir, interp.AllowedPaths([]string{dir}))
assert.Equal(t, 1, code)
assert.Contains(t, stderr, "tail:")
```

### Command-specific test wrapper

To avoid repeating `interp.AllowedPaths([]string{dir})` on every `runScript` call, define a
command-specific wrapper at the top of `builtin_$ARGUMENTS_test.go`:

```go
func cmdRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
    t.Helper()
    return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}
```

Use this wrapper throughout the test file. Use `runScript` directly only when you need different or
no `AllowedPaths` (e.g. for access-denied tests).

Tests should be written to the following specifications:

- All implemented flags are exercised in at least one test
- Review RULES.md and write tests verifying that the rules are honored where possible, checking for runaway memory allocations, infinite loops / hangs, etc
- Use `os.DevNull` instead of hardcoded `/dev/null` so tests compile on all platforms
- For tests that are inherently platform-specific (symlinks, Windows reserved names, directory reads), create separate files with build tags:
  - `builtin_$ARGUMENTS_unix_test.go` with `//go:build unix` at the top
  - `builtin_$ARGUMENTS_windows_test.go` with `//go:build windows` at the top
- When writing tests that pipe through another builtin (e.g. `cat file | $ARGUMENTS`), account for that builtin's output behaviour. For example, the `cat` builtin uses `fmt.Fprintln` which adds a trailing `\n` to each line — a binary file piped through `cat` will have a `\n` appended that was not in the original file.
- Do **not** use `echo -n` — the `echo` builtin does not support the `-n` flag and will emit the literal string `-n ` instead of suppressing the newline. For empty or newline-free stdin, write an empty file via `setup.files` in a YAML scenario or create a temp file in the test setup.

Verify the tests build and all fail (since we have no implementation yet).

## Step 5: Implement the $ARGUMENTS command

**PARALLEL STEP**: This runs concurrently with Steps 3 and 4. No gate check needed — Step 2 being `completed` is sufficient.

Create `pkg/shell/interp/builtins/$ARGUMENTS.go` (`package builtins`) following the patterns in
the existing builtins (e.g. `cat.go`):

1. **Function signature**: `func builtin$ARGUMENTS(ctx context.Context, callCtx *CallContext, args []string) Result`. All builtins take `ctx` — check `ctx.Err()` before every read in any loop.
2. **I/O**: Write output via `callCtx.Out(s)` / `callCtx.Outf(format, ...)` and errors via `callCtx.Errf(format, ...)`. Do not use `os.Stdout`/`os.Stderr` directly.
3. **File access**: Open files via `callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)` — never `os.Open()`. This enforces the allowed-paths sandbox automatically.
4. **Return values**: Return `Result{}` for success and `Result{Code: 1}` for failure. Do not panic or return Go errors for user-facing failures.
5. **Flag parsing**: Use pflag. Any unregistered flag is automatically rejected. Register `-h`/`--help` and handle it per RULES.md. For string flags that may receive an empty value or a special prefix (e.g. `+N` offset syntax), detect whether the flag was explicitly provided using `fs.Changed("flagname")` rather than comparing the value to `""`. Using `*flagStr != ""` is wrong in these cases — an explicit `cmd -n ""` would silently fall through to the default instead of being rejected.
6. **Bounded reads**: Cap all buffer allocations; never allocate based on unclamped user input.

Register the command in `pkg/shell/interp/builtins/builtins.go`:
- Add an entry to the `registry` map: `"$ARGUMENTS": builtin$ARGUMENTS`

**Do not modify any other existing files** unless directly required by the registration step above.

## Step 6: Verify and Harden

**GATE CHECK**: Call TaskList. Steps 3, 4, AND 5 must all be `completed` before starting this step. Set this step to `in_progress` now.

Run the tests:

```bash
dda inv test --targets=./pkg/shell/interp
```

Fix any failures before finishing.

After the initial test suite is passing, write another round of tests focused on:

- 100% code coverage of the implementation
- Additional tests specific to the rules in RULES.md. For example, if the implementation passes user input into buffer allocations, ensure in tests that this input is clamped to an appropriate value and not passed as-is to the buffer.

## Step 7: Code review

**GATE CHECK**: Call TaskList. Step 6 must be `completed` before starting this step. Set this step to `in_progress` now.

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

## Step 8: Exploratory pentest

**GATE CHECK**: Call TaskList. Step 7 must be `completed` before starting this step. Set this step to `in_progress` now.

The agent binary does **not** have a `shell` subcommand — do not attempt `agent shell --command`. Instead, perform all pentest exercises as Go tests in a dedicated file:

`pkg/shell/interp/builtin_$ARGUMENTS_pentest_test.go` (`package interp_test`)

Use the command-specific wrapper (e.g. `cmdRun`) or `runScript` directly. Use `context.WithTimeout` on individual tests to catch hangs:

```go
func TestCmdPentestInfiniteSource(t *testing.T) {
    dir := t.TempDir()
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    // exercise the command ...
}
```

Run with `go test ./pkg/shell/interp -run TestCmdPentest -timeout 120s`. For any surprising result, check whether GNU coreutils behaves the same way before deciding whether to fix it — surprising-but-matching-GNU is documenting a known behaviour, not a bug.

### Integer edge cases
- `-n 0`, `-n 1`, `-n MaxInt32`, `-n MaxInt64`, `-n MaxInt64+1`, `-n 99999999999999999999`
- `-n -1`, `-n -9999999999` (should reject)
- `-n +0`, `-n +1`, `-n +MaxInt64`
- `-n ''`, `-n '   '` (empty / whitespace)
- Same set for `-c`

### Special files / infinite sources
- Command in default line mode on `/dev/zero`, `/dev/random` — note whether it errors fast or spins
- Same in `-c` (byte) mode — compare timing against `gtail` to confirm matching behaviour
- `/dev/null` (empty source), `/proc` or `/sys` files if on Linux

### Long lines
- Line of `maxLineBytes - 1` bytes (should succeed)
- Line of exactly `maxLineBytes` bytes (documents where the cap actually bites)
- Line of `maxLineBytes + 1` bytes (should fail)
- Two lines each near the cap; verify last-line selection is correct

### Memory / resource exhaustion
- `-n MaxInt32` on a small file (verifies clamping, not OOM)
- `-c MaxInt32` on a small file
- 200+ file arguments (verifies no FD leak)
- 1M-line file through last-N and +N offset modes (verifies ring buffer correctness at scale)

### Path and filename edge cases
- Absolute path, `../` traversal, `//double//slashes`, `/etc/././hosts`
- Non-existent file, directory as file, empty-string filename
- Filename starting with `-` (use `--` separator)
- Symlink to a regular file, dangling symlink, circular symlink
- Symlink to `/dev/zero` (same DoS check as direct special file)

### Flag and argument injection
- Unknown flags (`-f`, `--follow`, `--no-such-flag`): confirm exit 1 + stderr, not fatal error
- Flag value via word expansion: `for flag in -f; do cmd $flag file; done`
- `--` end-of-flags followed by flag-like filenames
- Multiple `-` (stdin) arguments

### Behavior matching
For any case where behaviour differs from expectation, run the equivalent `gtail` invocation and compare. Differences fall into three categories:
1. **Matches GNU** — document in a code comment, no code change needed
2. **Safer than GNU** — document; generally keep our behaviour
3. **Worse than GNU** — fix it
