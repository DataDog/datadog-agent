# Safe Shell Interpreter - Builtin Commands Risk Analysis

This document catalogues known security risks in the builtin commands implementation (`pkg/shell/interp/`) and their recommended mitigations. Risks are ordered by severity.

For the full builtin command reference, see [BUILTINS.md](BUILTINS.md).

---

## RISK-1: Unrestricted Sensitive File Reads

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

## RISK-2: Memory Exhaustion via Large File Reads

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

## RISK-3: Memory Exhaustion via `head -c` Buffer Allocation

**Severity:** High
**Affected commands:** `head`
**Code:** `builtin_head.go` line `buf := make([]byte, byteCount)`

`head -c N` allocates a buffer of exactly N bytes. No upper bound is enforced, so `head -c 2147483647 somefile` allocates ~2 GB immediately, causing OOM.

**Mitigation:** Cap the `-c` value to a reasonable maximum (e.g., 10 MB). Validate all parsed integer arguments against upper bounds.

---

## RISK-4: Special Device File Reads

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

## RISK-5: Resource Exhaustion via Recursive Traversal

**Severity:** High
**Affected commands:** `find`, `grep -r`, `ls -R`
**Code pattern:** `filepath.Walk(absSearchPath, ...)` with no entry count limit

`find /`, `grep -r pattern /`, and `ls -R /` traverse the entire filesystem tree, consuming excessive CPU and memory (file info allocations, output buffering). While the 30-second timeout provides a ceiling, the process can still consume significant resources during that window.

**Mitigation:** Enforce a default `maxdepth` for `find` when none is specified. Add a cap on the total number of entries visited by any recursive traversal. Consider restricting the starting paths for recursive operations.

---

## RISK-6: `cd` Enables Path Traversal for All Subsequent Commands

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

## RISK-7: Symlink Following in `find` Walk

**Severity:** High
**Affected commands:** `find`
**Code:** `filepath.Walk(absSearchPath, ...)` in `builtin_find.go`

`filepath.Walk` follows symlinks. A symlink to `/` or to a deeply nested directory could cause `find` to traverse far beyond the intended search path. Circular symlinks could create very long traversals (Go's `filepath.Walk` does not protect against cycles).

Additionally, the symlink detection code for `-type l` is ineffective: `filepath.Walk` provides the stat of the symlink target (not the symlink itself), so the check `info.Mode()&os.ModeSymlink != 0` in the walk callback will always be false. The code attempts to call `os.Lstat` as a workaround but does so after the `info.Mode()&os.ModeSymlink` check, meaning the Lstat result is only used when the Walk info already shows a symlink (which it won't).

**Mitigation:** Replace `filepath.Walk` with `filepath.WalkDir` and use `d.Type()` to detect symlinks without following them. Don't traverse into symlinked directories during recursive walks.

---

## RISK-8: Glob Expansion Bomb

**Severity:** Medium
**Affected commands:** All (via `expandGlob` in expand.go), `for` loops
**Code:** `filepath.Glob(absPattern)` in `expand.go`

Glob expansion is unbounded. Patterns like `/*/*/*/*/*` or `for f in /*/*/*; do ...` eagerly expand all matches into a `[]string` before processing, potentially generating millions of entries and consuming gigabytes of memory.

**Mitigation:** Limit the number of glob matches returned (e.g., 10,000). Return an error if the limit is exceeded.

---

## RISK-9: Filesystem Probing via `test` / `[` and `find`

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

**Mitigation:** The file allowlist should also cover `os.Stat` and `os.Lstat` calls, not just `os.Open`/`os.ReadFile`.

---

## RISK-10: Unbounded Integer Arguments

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

## RISK-11: Missing `defer` on File Handles

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

## RISK-12: `grep -r` Without Default File Filters

**Severity:** Medium
**Affected commands:** `grep -r`

`grep -r pattern .` recursively searches all files including binary files, very large files, and special files. Without `--include` or `--exclude`, it opens every file under the search path.

**Mitigation:** Consider adding a default `--exclude` for binary file extensions or adding a binary file detection heuristic (check for null bytes in the first N bytes).

---

## RISK-13: `cat -` / `head -` / `tail` Stdin Blocking

**Severity:** Low
**Affected commands:** `cat`, `head`, `tail`, `sort`, `wc`, `uniq`, `grep`

When `-` is passed as a filename (or no files are given), these commands read from stdin. If stdin is not connected to a pipe, the command blocks indefinitely until the 30-second timeout.

**Mitigation:** The 30-second timeout provides adequate protection. No additional mitigation needed unless shorter response times are required.

---

## RISK-14: `find -mtime` / `find -mmin` Temporal Information Leak

**Severity:** Low
**Affected commands:** `find`

`find -mtime` and `find -mmin` reveal file modification timestamps without reading contents. This can leak information about system activity patterns:

```
find /var/log -mmin -5    # reveals recently-active logs
find /tmp -mtime -1       # reveals recent temp file activity
```

**Mitigation:** Covered by the file allowlist (restricts which paths can be traversed).

---

## RISK-15: Pipe Chain Resource Usage

**Severity:** Low
**Affected commands:** Pipe operator (`|`)
**Code:** `r.pipe()` in `interp.go` creates `os.Pipe()` + goroutine per pipe

Each pipe creates a file descriptor pair and a goroutine. Deeply nested pipes (`cmd1 | cmd2 | ... | cmdN`) create N file descriptors and N goroutines. While the 30-second timeout bounds overall duration, a sufficiently long chain could briefly consume significant file descriptors.

**Mitigation:** Limit the depth of pipe chains (e.g., max 10 stages). The 30-second timeout provides partial protection.

---

## RISK-16: TOCTOU in `cd` Between Stat and Assignment

**Severity:** Low
**Affected commands:** `cd`
**Code:** `os.Stat(target)` → `r.dir = target` in `builtin.go`

There is a race between validating that a directory exists and setting it as the working directory. A directory could be deleted or replaced with a symlink between the check and the assignment.

**Mitigation:** Minimal real-world risk since `cd` only changes an in-memory variable and doesn't call `chdir()`. No action required.

---

## Summary Table

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
