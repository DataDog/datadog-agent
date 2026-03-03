# Safe Shell Interpreter - Builtin Commands Reference

## Context

This document describes all builtin commands available in the safe shell interpreter (`pkg/shell/interp/`). The interpreter is designed for the Private Action Runner (PAR) to execute shell scripts from AI agents without ever invoking host binaries or `/bin/sh`. It parses shell scripts via `mvdan.cc/sh/v3` and directly interprets the AST, eliminating the TOCTOU (Time-of-Check Time-of-Use) gap that verify-then-exec approaches have.

---

## Architecture & Safety Overview

### Execution Model
- **No host command execution** - The interpreter NEVER invokes external binaries. All 18 commands are implemented as pure Go builtins.
- **AST-level enforcement** - Shell scripts are parsed into an AST and only allowed constructs are executed.
- **Literal command names required** - Command names must be literal strings; dynamic command names (e.g. from variables) are rejected.

### Global Safety Controls

| Control | Default | Description |
|---------|---------|-------------|
| Execution timeout | 30 seconds | Configurable via `executor.WithTimeout()` |
| Output size limit | 1 MB | Configurable via `executor.WithMaxOutputSize()`. Applies separately to stdout and stderr. |
| Environment allowlist | 9 variables | Only `PATH`, `HOME`, `LANG`, `LC_ALL`, `TERM`, `TMPDIR`, `TZ`, `USER`, `LOGNAME` are passed through. `PATH` is hardcoded to `/usr/bin:/bin:/usr/local/bin`. |

### Blocked Shell Constructs
- Variable assignments (`VAR=value`)
- Redirections (`>`, `<`, `>>`, `2>`, etc.)
- Command substitution (`$(...)`, backticks)
- Process substitution (`<(...)`, `>(...)`)
- Arithmetic expansion (`$((expr))`)
- Brace expansion (`{a,b,c}`)
- Extended globs (`@()`, `+()`, etc.)
- Complex parameter expansion (`${VAR:-default}`, etc.)
- `if`/`while`/`until`/`case`/`select` statements
- Function declarations
- Subshells, blocks (`{ }`)
- Background execution (`&`), coprocesses
- `eval`, `exec`, `source`, `.`, `trap`

### Allowed Shell Constructs
- Simple commands (builtin + arguments)
- Pipes (`|`)
- Logical operators (`&&`, `||`)
- For-in loops (`for VAR in LIST; do ... done`)
- Negation (`! COMMAND`)
- Glob expansion (`*`, `?`, `[...]`) in arguments
- For-loop variable expansion (`$VAR`)
- Single and double quotes (no expansion in either)

---

## Builtin Commands

### 1. `echo` - Print text to stdout

**Source:** `builtin.go:93-167`
**Safety:** Read-only, no filesystem access. Always succeeds (exit code 0).

| Flag | Description |
|------|-------------|
| `-n` | Do not output trailing newline |
| `-e` | Interpret backslash escape sequences |
| `-E` | Disable interpretation of escape sequences (default) |

**Escape sequences** (with `-e`): `\n` (newline), `\t` (tab), `\\` (backslash), `\a` (alert), `\b` (backspace), `\r` (carriage return), `\v` (vertical tab), `\c` (stop output immediately).

---

### 2. `true` - Return success

**Source:** `builtin.go:53`
**Safety:** No side effects. Sets exit code to 0.

---

### 3. `false` - Return failure

**Source:** `builtin.go:55`
**Safety:** No side effects. Sets exit code to 1.

---

### 4. `test` / `[` - Evaluate conditional expressions

**Source:** `builtin.go:169-260`
**Safety:** Read-only. File tests use `os.Stat` (follows symlinks). No write access.

`[` is an alias for `test` that requires a trailing `]` argument.

#### Unary operators (file tests)

| Operator | Description |
|----------|-------------|
| `-e FILE` | True if FILE exists |
| `-f FILE` | True if FILE exists and is a regular file |
| `-d FILE` | True if FILE exists and is a directory |
| `-s FILE` | True if FILE exists and has size > 0 |

#### Unary operators (string tests)

| Operator | Description |
|----------|-------------|
| `-z STRING` | True if STRING is empty |
| `-n STRING` | True if STRING is non-empty |

#### Binary operators

| Operator | Description |
|----------|-------------|
| `=` | String equality |
| `!=` | String inequality |
| `-eq` | Integer equal |
| `-ne` | Integer not equal |
| `-lt` | Integer less than |
| `-le` | Integer less than or equal |
| `-gt` | Integer greater than |
| `-ge` | Integer greater than or equal |

#### Special

| Syntax | Description |
|--------|-------------|
| `! EXPR` | Negation (prefix any expression) |
| `STRING` | True if STRING is non-empty (single argument) |

---

### 5. `break` - Exit loop

**Source:** `builtin.go:262-267`
**Safety:** No side effects. Only valid inside `for` loops; errors if used outside a loop.

---

### 6. `continue` - Skip to next loop iteration

**Source:** `builtin.go:269-274`
**Safety:** No side effects. Only valid inside `for` loops; errors if used outside a loop.

---

### 7. `exit` - Exit with status code

**Source:** `builtin.go:276-286`
**Safety:** Terminates script execution. No side effects beyond setting exit code.

| Syntax | Description |
|--------|-------------|
| `exit` | Exit with current exit code |
| `exit N` | Exit with code N |

---

### 8. `cd` - Change working directory

**Source:** `builtin.go:288-329`
**Safety:** Changes the interpreter's working directory (in-memory state only). Does NOT call `chdir()` on the host process. Validates target exists and is a directory.

| Flag | Description |
|------|-------------|
| `-L` | Accepted but ignored (logical mode) |
| `-P` | Accepted but ignored (physical mode) |

| Syntax | Description |
|--------|-------------|
| `cd` | Change to `$HOME` |
| `cd PATH` | Change to PATH (absolute or relative) |

---

### 9. `pwd` - Print working directory

**Source:** `builtin.go:331-342`
**Safety:** Read-only. Prints the interpreter's current directory.

| Flag | Description |
|------|-------------|
| `-L` | Accepted but ignored |
| `-P` | Accepted but ignored |

---

### 10. `cat` - Concatenate and display files

**Source:** `builtin_cat.go`
**Safety:** Read-only file access via `os.Open`. Reads from stdin if no files given.

| Flag | Long form | Description |
|------|-----------|-------------|
| `-n` | `--number` | Number all output lines |
| `-E` | `--show-ends` | Display `$` at end of each line |
| `-s` | `--squeeze-blank` | Suppress repeated empty lines |
| `--` | | End of flags |

**Special:** `-` as a filename reads from stdin.

---

### 11. `ls` - List directory contents

**Source:** `builtin_ls.go`
**Safety:** Read-only. Uses `os.ReadDir` / `os.Lstat`. No write access.

| Flag | Description |
|------|-------------|
| `-l` | Long format (permissions, size, modification time, name) |
| `-a` | Show hidden files (names starting with `.`) |
| `-h` | Human-readable file sizes (K, M, G, T) |
| `-t` | Sort by modification time (newest first) |
| `-R` | Recursive listing |
| `-r` | Reverse sort order |
| `-1` | One entry per line |
| `-d` | List directory entries themselves, not their contents |
| `--` | End of flags |

---

### 12. `head` - Output first part of files

**Source:** `builtin_head.go`
**Safety:** Read-only file access via `os.Open`. Reads from stdin if no files given.

| Flag | Long form | Description |
|------|-----------|-------------|
| `-n N` | `--lines=N` | Output first N lines (default: 10) |
| `-c N` | `--bytes=N` | Output first N bytes |
| `--` | | End of flags |

**Multi-file:** Shows `==> filename <==` headers between files when multiple files are given.

---

### 13. `tail` - Output last part of files

**Source:** `builtin_tail.go`
**Safety:** Read-only. Reads entire file into memory via `os.ReadFile`. Reads from stdin if no files given.

| Flag | Long form | Description |
|------|-----------|-------------|
| `-n N` | `--lines=N` | Output last N lines (default: 10) |
| `-n +N` | | Output starting from line N (1-indexed) |
| `-c N` | `--bytes=N` | Output last N bytes |
| `+N` | | Shorthand for `-n +N` |
| `--` | | End of flags |

**Multi-file:** Shows `==> filename <==` headers between files when multiple files are given.

---

### 14. `find` - Search for files in a directory hierarchy

**Source:** `builtin_find.go`
**Safety:** Read-only. Uses `filepath.Walk` and `os.Stat`/`os.Lstat`/`os.ReadDir`. No write access.

#### Depth control

| Option | Description |
|--------|-------------|
| `-maxdepth N` | Descend at most N levels below search path |
| `-mindepth N` | Do not apply tests at levels less than N |

#### Predicates (all support negation with `-not` or `!`)

| Predicate | Description |
|-----------|-------------|
| `-name PATTERN` | Filename matches glob pattern |
| `-iname PATTERN` | Case-insensitive filename glob |
| `-type f\|d\|l` | File type: regular file, directory, or symlink |
| `-size [+-]N[cwkMG]` | File size. Units: `c`=bytes, `w`=2-byte words, `k`=KiB, `M`=MiB, `G`=GiB. Default: 512-byte blocks. Prefix: `+`=greater, `-`=less, none=exact |
| `-mtime [+-]N` | Modification time in days. `+N`=older, `-N`=newer |
| `-mmin [+-]N` | Modification time in minutes |
| `-path PATTERN` | Full path matches glob pattern |
| `-empty` | File has zero size or directory has no entries |
| `-newer FILE` | Newer than reference file |

#### Actions

| Action | Description |
|--------|-------------|
| `-print` | Print pathname (default, accepted but ignored since it's always the behavior) |

**Predicates are ANDed together.** All predicates must match for a file to be printed.

---

### 15. `grep` - Search file contents for patterns

**Source:** `builtin_grep.go`
**Safety:** Read-only file access. Uses Go's `regexp` package for pattern matching. Reads from stdin if no files given.

#### Matching options

| Flag | Description |
|------|-------------|
| `-i` | Case-insensitive matching |
| `-v` | Invert match (select non-matching lines) |
| `-w` | Match whole words only (wraps pattern with `\b`) |
| `-E` | Extended regex (accepted, Go regex is already extended) |
| `-F` | Fixed strings (no regex, `regexp.QuoteMeta` applied) |
| `-e PATTERN` | Specify pattern explicitly (can be used multiple times for OR) |

#### Output options

| Flag | Description |
|------|-------------|
| `-n` | Show line numbers |
| `-c` | Count matching lines only |
| `-l` | Print only names of files with matches |
| `-m N` | Stop after N matches per file |

#### Context options

| Flag | Description |
|------|-------------|
| `-A N` | Print N lines after each match |
| `-B N` | Print N lines before each match |
| `-C N` | Print N lines before and after each match |

#### Recursive search

| Flag | Description |
|------|-------------|
| `-r` | Recursive search through directories |
| `--include=PATTERN` | Only search files matching glob |
| `--exclude=PATTERN` | Skip files matching glob |
| `--exclude-dir=PATTERN` | Skip directories matching glob |

**Combined flags:** Short boolean flags can be combined (e.g., `-inr`).

---

### 16. `wc` - Word, line, and byte count

**Source:** `builtin_wc.go`
**Safety:** Read-only. Reads entire file into memory via `os.ReadFile`. Reads from stdin if no files given.

| Flag | Description |
|------|-------------|
| `-l` | Count lines |
| `-w` | Count words |
| `-c` | Count bytes |

**Default:** If no flags are given, all three counts are shown.
**Multi-file:** Shows a `total` line when multiple files are given.

---

### 17. `sort` - Sort lines of text

**Source:** `builtin_sort.go`
**Safety:** Read-only. Reads entire input into memory. Reads from stdin if no files given.

| Flag | Description |
|------|-------------|
| `-r` | Reverse sort order |
| `-n` | Numeric sort |
| `-u` | Unique (remove duplicate lines after sorting) |
| `-f` | Fold case (case-insensitive sort) |
| `-h` | Human-numeric sort (e.g., 2K, 1G) |
| `-k N` | Sort by field N (1-indexed). Supports `N,M` syntax (only start field used) |
| `-t SEP` | Field separator character |
| `--` | End of flags |

**Combined flags:** Short boolean flags can be combined (e.g., `-rn`).
**Human-numeric units:** K, M, G, T (base 1024).

---

### 18. `uniq` - Report or omit repeated lines

**Source:** `builtin_uniq.go`
**Safety:** Read-only. Reads from a single file or stdin. Requires sorted input for correct behavior.

| Flag | Description |
|------|-------------|
| `-c` | Prefix lines with count of occurrences |
| `-d` | Only print duplicate lines |
| `-i` | Ignore case when comparing |
| `--` | End of flags |

**Input:** Reads at most one file argument, or stdin if none given.

---

## Explicitly Blocked Commands

The following shell builtins are explicitly listed in `verifier/commands.go` as blocked, providing clear error messages if attempted:

| Command | Reason |
|---------|--------|
| `eval` | Arbitrary code execution |
| `exec` | Replace process with external binary |
| `source` | Load and execute external script file |
| `.` (dot) | Alias for `source` |
| `trap` | Signal handler manipulation |

Any command not in the 18-command builtin list is also rejected with `"command X is not allowed"`.

---

## Safety Classification Summary

| Category | Commands | Risk Level |
|----------|----------|------------|
| **No side effects** | `echo`, `true`, `false`, `test`/`[`, `break`, `continue`, `exit`, `pwd` | None |
| **Read-only filesystem** | `cat`, `ls`, `head`, `tail`, `find`, `grep`, `wc`, `sort`, `uniq` | Low - can read any file accessible to the agent process |
| **Interpreter state change** | `cd` | None - only changes in-memory working directory |

All commands are **read-only with respect to the filesystem**. No builtin can write, delete, or modify files. The only state mutation is `cd` changing the interpreter's working directory (an in-memory variable, not a host `chdir()`).

---

---

## Security Risk Analysis

This section catalogues known risks in the builtin commands implementation and their recommended mitigations. Risks are grouped by category and ordered by severity within each group.

### RISK-1: Unrestricted Sensitive File Reads

**Severity:** Critical
**Affected commands:** `cat`, `head`, `tail`, `grep`, `wc`, `sort`, `uniq`
**Code pattern:** `os.Open(path)`, `os.ReadFile(path)`

All file-reading builtins can open any file accessible to the agent process, including `/etc/shadow`, private SSH keys, API tokens, cloud credentials, and Datadog agent secrets.

```
cat /etc/shadow
grep -r password /etc/
head -c 4096 /root/.ssh/id_rsa
sort /proc/1/environ
```

**Mitigation:** The planned file allowlist must wrap every `os.Open` and `os.ReadFile` call. Until implemented, the process must run with minimal filesystem permissions.

---

### RISK-2: Memory Exhaustion via Large File Reads

**Severity:** Critical
**Affected commands:** `tail`, `wc`, `sort`, `grep`
**Code pattern:** `os.ReadFile(path)`, `readLines(reader)`, `bufio.Scanner` into `[]string`

Several builtins read entire files into memory before processing:
- `tail` → `os.ReadFile(path)` (builtin_tail.go)
- `wc` → `os.ReadFile(path)` (builtin_wc.go)
- `sort` → `readLines(reader)` accumulates all lines in `[]string` (builtin_sort.go)
- `grep` → scans all lines into `[]string` for context support (builtin_grep.go)

A multi-GB file (e.g., `tail /var/log/huge.log` or `sort /dev/zero`) can trigger OOM because the 1 MB output limit only caps stdout/stderr, not the amount of data read into memory.

**Mitigation:** Add a maximum input file size check (e.g., 10 MB) before reading. Reject or truncate files that exceed the limit. Alternatively, refactor `tail` to use seek-based reading for the last-N-lines case.

---

### RISK-3: Memory Exhaustion via `head -c` Buffer Allocation

**Severity:** High
**Affected commands:** `head`
**Code:** `builtin_head.go` line `buf := make([]byte, byteCount)`

`head -c N` allocates a buffer of exactly N bytes. No upper bound is enforced, so `head -c 2147483647 somefile` allocates ~2 GB immediately, causing OOM.

**Mitigation:** Cap the `-c` value to a reasonable maximum (e.g., 10 MB). Validate all parsed integer arguments against upper bounds.

---

### RISK-4: Special Device File Reads

**Severity:** High
**Affected commands:** `cat`, `head`, `tail`, `grep`, `wc`, `sort`, `uniq`
**Code pattern:** `os.Open(path)` — no check for file type before reading

Reading from device files can cause hangs or unbounded reads:
- `/dev/zero`, `/dev/urandom` → infinite data, fills memory
- `/dev/kmsg` → leaks kernel log messages
- `/proc/kcore` → can expose kernel memory
- `/proc/self/environ` → leaks all environment variables of the agent process

```
cat /dev/urandom | head -c 100   # works but the left side reads infinitely
sort /dev/zero                   # OOM
```

**Mitigation:** The file allowlist should exclude device files. Additionally, check `info.Mode().IsRegular()` before opening files in all builtins, or reject paths under `/dev/`, `/proc/`, and `/sys/`.

---

### RISK-5: Resource Exhaustion via Recursive Traversal

**Severity:** High
**Affected commands:** `find`, `grep -r`, `ls -R`
**Code pattern:** `filepath.Walk(absSearchPath, ...)` with no entry count limit

`find /`, `grep -r pattern /`, and `ls -R /` traverse the entire filesystem tree, consuming excessive CPU and memory (file info allocations, output buffering). While the 30-second timeout provides a ceiling, the process can still consume significant resources during that window.

**Mitigation:** Enforce a default `maxdepth` for `find` when none is specified. Add a cap on the total number of entries visited by any recursive traversal. Consider restricting the starting paths for recursive operations.

---

### RISK-6: `cd` Enables Path Traversal for All Subsequent Commands

**Severity:** High
**Affected commands:** `cd` (amplifies all file-reading builtins)

`cd` changes the interpreter's working directory without restriction. This lets an attacker bypass any directory-based heuristics:

```
cd /etc && cat shadow
cd /root && cat .ssh/id_rsa
cd / && find . -name "*.pem"
```

**Mitigation:** The file allowlist must apply regardless of the current working directory (i.e., resolve all paths to absolute before checking). Optionally, restrict `cd` to a set of allowed directories.

---

### RISK-7: Symlink Following in `find` Walk

**Severity:** High
**Affected commands:** `find`
**Code:** `filepath.Walk(absSearchPath, ...)` in `builtin_find.go`

`filepath.Walk` follows symlinks. A symlink to `/` or to a deeply nested directory could cause `find` to traverse far beyond the intended search path. Circular symlinks could create very long traversals (Go's `filepath.Walk` does not protect against cycles).

Additionally, the symlink detection code for `-type l` is ineffective: `filepath.Walk` provides the stat of the symlink target (not the symlink itself), so the check `info.Mode()&os.ModeSymlink != 0` in the walk callback will always be false. The code attempts to call `os.Lstat` as a workaround but does so after the `info.Mode()&os.ModeSymlink` check, meaning the Lstat result is only used when the Walk info already shows a symlink (which it won't).

**Mitigation:** Replace `filepath.Walk` with `filepath.WalkDir` and use `d.Type()` to detect symlinks without following them. Don't traverse into symlinked directories during recursive walks.

---

### RISK-8: Glob Expansion Bomb

**Severity:** Medium
**Affected commands:** All (via `expandGlob` in expand.go), `for` loops
**Code:** `filepath.Glob(absPattern)` in `expand.go`

Glob expansion is unbounded. Patterns like `/*/*/*/*/*` or `for f in /*/*/*; do ...` eagerly expand all matches into a `[]string` before processing, potentially generating millions of entries and consuming gigabytes of memory.

**Mitigation:** Limit the number of glob matches returned (e.g., 10,000). Return an error if the limit is exceeded.

---

### RISK-9: Filesystem Probing via `test` / `[` and `find`

**Severity:** Medium
**Affected commands:** `test`, `[`, `find`, `ls`
**Code:** `os.Stat(path)` in `evalTest`, `find -newer`, `ls`

These commands allow probing whether files exist, their types, sizes, and modification times—without reading file contents:

```
test -f /root/.ssh/id_rsa && echo "SSH key exists"
test -s /etc/shadow && echo "shadow file is non-empty"
find /home -name ".aws" -type d
find / -name "*.pem" -newer /etc/hostname
ls -lR /etc/
```

This information disclosure helps attackers identify targets for subsequent exploitation.

**Mitigation:** The file allowlist should also cover `os.Stat` and `os.Lstat` calls, not just `os.Open`.

---

### RISK-10: Unbounded Integer Arguments

**Severity:** Medium
**Affected commands:** `head`, `tail`, `grep`, `sort`, `find`
**Code pattern:** `strconv.Atoi(args[i])` with no range validation

Multiple builtins parse integers from arguments without upper-bound validation:
- `head -n 999999999` → reads ~1 billion lines
- `tail -c 999999999` → reads entire file, then slices
- `grep -m 999999999` → effectively unlimited matches
- `grep -A 999999999` → accumulates massive context buffers
- `find -maxdepth 999999999` → effectively unlimited depth

**Mitigation:** Validate all parsed integer arguments against reasonable upper bounds immediately after parsing.

---

### RISK-11: Missing `defer` on File Handles

**Severity:** Medium
**Affected commands:** `cat`, `head`, `grep` (non-recursive path)
**Code:** `file.Close()` called explicitly instead of `defer`

In `builtin_cat.go`, `builtin_head.go`, and the non-recursive `grep` path, file handles are closed with explicit `file.Close()` calls after use. If the processing function (e.g., `catOutput`, `headOutput`) panics, the file descriptor leaks.

```go
// builtin_cat.go — fd leak if catOutput panics
file, err := os.Open(path)
if err != nil { ... }
if err := catOutput(file); err != nil { hasError = true }
file.Close()  // skipped on panic
```

**Mitigation:** Use `defer file.Close()` consistently across all builtins. The `grep` recursive path already does this correctly.

---

### RISK-12: `grep -r` Without Default File Filters

**Severity:** Medium
**Affected commands:** `grep -r`

`grep -r pattern .` recursively searches all files including binary files, very large files, and special files. Without `--include` or `--exclude`, it opens every file under the search path.

**Mitigation:** Consider adding a default `--exclude` for binary file extensions or adding a binary file detection heuristic (check for null bytes in the first N bytes).

---

### RISK-13: `cat -` / `head -` / `tail` Stdin Blocking

**Severity:** Low
**Affected commands:** `cat`, `head`, `tail`, `sort`, `wc`, `uniq`, `grep`

When `-` is passed as a filename (or no files are given), these commands read from stdin. If stdin is not connected to a pipe, the command blocks indefinitely until the 30-second timeout.

**Mitigation:** The 30-second timeout provides adequate protection. No additional mitigation needed unless shorter response times are required.

---

### RISK-14: `find -mtime` / `find -mmin` Temporal Information Leak

**Severity:** Low
**Affected commands:** `find`

`find -mtime` and `find -mmin` reveal file modification timestamps without reading contents. This can leak information about system activity patterns:

```
find /var/log -mmin -5    # reveals recently-active logs
find /tmp -mtime -1       # reveals recent temp file activity
```

**Mitigation:** Covered by the file allowlist (restricts which paths can be traversed).

---

### RISK-15: Pipe Chain Resource Usage

**Severity:** Low
**Affected commands:** Pipe operator (`|`)
**Code:** `r.pipe()` in `interp.go` creates `os.Pipe()` + goroutine per pipe

Each pipe creates a file descriptor pair and a goroutine. Deeply nested pipes (`cmd1 | cmd2 | ... | cmdN`) create N file descriptors and N goroutines. While the 30-second timeout bounds overall duration, a sufficiently long chain could briefly consume significant file descriptors.

**Mitigation:** Limit the depth of pipe chains (e.g., max 10 stages). The 30-second timeout provides partial protection.

---

### RISK-16: TOCTOU in `cd` Between Stat and Assignment

**Severity:** Low
**Affected commands:** `cd`
**Code:** `os.Stat(target)` → `r.dir = target` in `builtin.go`

There is a race between validating that a directory exists and setting it as the working directory. A directory could be deleted or replaced with a symlink between the check and the assignment.

**Mitigation:** Minimal real-world risk since `cd` only changes an in-memory variable and doesn't call `chdir()`. No action required.

---

### Summary Table

| ID | Risk | Severity | Affected Commands | Mitigated by File Allowlist |
|----|------|----------|-------------------|-----------------------------|
| RISK-1 | Unrestricted sensitive file reads | Critical | cat, head, tail, grep, wc, sort, uniq | ✅ Yes |
| RISK-2 | Memory exhaustion via large file reads | Critical | tail, wc, sort, grep | ❌ No — needs input size cap |
| RISK-3 | Memory exhaustion via head -c allocation | High | head | ❌ No — needs argument validation |
| RISK-4 | Special device file reads | High | cat, head, tail, grep, wc, sort, uniq | ✅ Partially — must exclude /dev, /proc, /sys |
| RISK-5 | Resource exhaustion via recursive traversal | High | find, grep -r, ls -R | ❌ No — needs entry count limit |
| RISK-6 | cd enables path traversal | High | cd | ✅ Yes — if allowlist uses absolute paths |
| RISK-7 | Symlink following in find | High | find | ❌ No — needs WalkDir + no-follow |
| RISK-8 | Glob expansion bomb | Medium | all (via expandGlob) | ❌ No — needs match count limit |
| RISK-9 | Filesystem probing via test/find/ls | Medium | test, [, find, ls | ✅ Partially — allowlist must also wrap Stat |
| RISK-10 | Unbounded integer arguments | Medium | head, tail, grep, sort, find | ❌ No — needs range validation |
| RISK-11 | Missing defer on file handles | Medium | cat, head, grep | ❌ No — code fix needed |
| RISK-12 | grep -r without default file filters | Medium | grep -r | ❌ No — needs binary exclusion |
| RISK-13 | Stdin blocking | Low | cat, head, tail, sort, wc, uniq, grep | ✅ Yes — 30s timeout |
| RISK-14 | Temporal information leak | Low | find | ✅ Yes |
| RISK-15 | Pipe chain resource usage | Low | pipe operator | ❌ No — needs depth limit |
| RISK-16 | TOCTOU in cd | Low | cd | ✅ N/A — no real exploit |

---

## Source Files

| File | Description |
|------|-------------|
| `pkg/shell/interp/interp.go` | Main interpreter, AST walker, pipe/loop execution |
| `pkg/shell/interp/builtin.go` | Builtin registry + echo, test, break, continue, exit, cd, pwd |
| `pkg/shell/interp/builtin_cat.go` | `cat` implementation |
| `pkg/shell/interp/builtin_ls.go` | `ls` implementation |
| `pkg/shell/interp/builtin_head.go` | `head` implementation |
| `pkg/shell/interp/builtin_tail.go` | `tail` implementation |
| `pkg/shell/interp/builtin_find.go` | `find` implementation |
| `pkg/shell/interp/builtin_grep.go` | `grep` implementation |
| `pkg/shell/interp/builtin_wc.go` | `wc` implementation |
| `pkg/shell/interp/builtin_sort.go` | `sort` implementation |
| `pkg/shell/interp/builtin_uniq.go` | `uniq` implementation |
| `pkg/shell/interp/expand.go` | Word/variable/glob expansion with safety restrictions |
| `pkg/shell/interp/command.go` | Command rejection for non-builtins |
| `pkg/shell/verifier/commands.go` | Blocked builtins list |
| `pkg/shell/executor/executor.go` | High-level executor with timeout and output limits |
| `pkg/privateactionrunner/bundles/ddagent/shell/run_shell.go` | PAR integration with environment filtering |
