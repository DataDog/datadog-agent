# Shell Features Reference

This document lists every shell feature and whether it is supported (✅) or blocked (❌).
Blocked features are rejected before execution with exit code 2.

The in-shell `help` command mirrors these feature categories: run `help` for a concise supported/unsupported summary plus allowed and selectively elevatable commands (the `Elevatable commands` section remains visible when empty, with an explicit notice that `sudo` is unavailable), or `help <feature|command>` for details about a specific feature or command.

## Library policy checks

`(*interp.Runner).Check(ctx, script)` provides a non-executing authorization
preview using the runner's existing configuration and current shell state.
`interp.CheckRemote(ctx, script, RemotePolicy)` uses the same checker with
explicit effective grants for another host, without constructing a runner or
consulting local filesystem state. The embedding application authenticates the
caller, resolves the target, and computes these grants before calling rshell.
`CheckResult` contains an overall `Allowed` boolean and `Status`, a `Commands`
array with the same fields and source line/column, script-wide `Issues`, and
configuration `Warnings`. Issue codes and JSON field names are declared in
[`interp/check.go`](interp/check.go). Parse and policy failures are report data;
cancellation, timeouts, invalid runners, and malformed remote configuration use
the Go error return.

| Surface | Static check |
|---|---|
| Shell syntax | Same `ParseScript` size limit and restricted AST validation as execution |
| Commands | Exact `AllowedCommands`, builtin registration, remediation-only metadata, selective elevation, and prohibition of elevation in pipelines |
| Flags | Same builtin flag factories, normalization, and parsing as execution; handler-level semantic checks and embedded AWK/sed/jq programs are not validated |
| Paths | Explicit redirects and modeled builtin operands; local checks use sandbox metadata for containment and write restrictions; remote checks use only lexical grant parsing, containment, and access precedence, leaving target metadata indeterminate |
| Systemd | Exact unit/action grants for `systemctl` and `journalctl`; no backend calls or target availability checks |
| Variables | Configured/current environment and simple sequential assignments, with quoting and field splitting; values changed by conditional branches, loops, or `read` may be unknown |
| Control flow | Every syntactic command site is checked once, including branches that might not run; subshell/pipeline assignments do not leak into the parent |

`allowed` means the modeled authorization checks passed. `denied` means a
concrete syntax, flag, or policy refusal was found. `indeterminate` means some
required check could not be completed without runtime information. `Allowed`
is true only for `allowed`; a denial takes precedence over uncertainty.

Globs, brace expansions, command-substitution results, and relative paths after
`cd` are conservatively indeterminate. `cd` operands containing `..` also require
execution's intermediate-directory and symlink checks in both `-L` and `-P` modes.
Nested substitutions are checked but
never evaluated, including the implicit `cat` policy for `$(<file)`. The checker
does not read checksum manifests or xargs input: `sha256sum -c` and `xargs`
therefore report incomplete checks, while checking explicit manifest/input
paths and the fixed xargs command's policy. `find` checks starting paths but
leaves expressions and nested commands unresolved. Embedded AWK/sed programs,
AWK program files and assignment operands, jq operands, and general `test` expressions also require
runtime checks. Unmodeled future builtins default to indeterminate. The checker
does not infer filesystem paths from ordinary text arguments such as echo
messages or grep patterns.

Checks never consume stdin, write stdout/stderr, execute builtin handlers,
evaluate substitutions, open redirects, call elevation/systemd backends, or
change runner state. Only `Runner.Check` may inspect sandboxed filesystem metadata. Reports
are bounded to 16,384 command sites and 128 levels of command nesting; word
expansion uses bounded per-word and cumulative byte budgets and respects the
runner's execution timeout and caller cancellation. Assignment storage totals
are maintained incrementally, with cancellation checked during assignment analysis.

Remote checks accept `AllowedCommands` and `ElevatableCommands` with `rshell:`
prefixes, absolute `AllowedPaths` with `:ro`/`:rw` modes, `Mode` (default read-only),
exact `AllowedSystemServices` unit/action grants, explicit `Env` pairs (default
empty), optional absolute `Dir`, and `Timeout` (zero defaults to five seconds;
negative is an error; earlier context deadlines win). There is no elevation
callback, stream, or backend input. No local roots, symlinks, files, `/proc`, cwd,
or process environment are inspected. The policy-only path representation has
no executable filesystem operations and cannot weaken a normal runner's sandbox.

Remote missing command, path, write, mode, elevation, and service grants are
denied when determinable. A lexically matching path remains indeterminate:
the target must verify root availability, symlink containment, file types,
hard-link restrictions, and OS permissions. Root-dependent builtin gates, such
as `tee` requiring an available writable root even for `--help`, also remain
indeterminate. Relative paths without `Dir` cannot inherit a local cwd or the
first configured grant; `$PWD` is unknown, and configured `$ALLOWED_PATHS`
requires target root discovery. Original path operands are retained in issues;
local missing-root warnings are never emitted.

Remote grants use execution's pure suffix parser and most-specific root
selection. Malformed grants are rejected; colons are reserved for one terminal
mode suffix or a Windows volume prefix. POSIX literal directories ending in
`:ro`/`:rw` are a target-state ambiguity, not a reason to inspect local roots.
The checker and target must use the same rshell version and platform conventions;
path syntax and builtin registration follow the checking build's platform.
See [the remote API example and inputs](README.md#preview-authorization-for-a-remote-host).

This API does not predict exit codes, existence/accessibility of files, platform
availability, host privileges, service state, or termination. A denied site
might be unreachable at runtime; an allowed script can still fail. Checks are
advisory snapshots, so execution must always use the normal policy-enforcing
`Run` method against the target's current policies and state. No CLI dry-run flag
or privileged-helper protocol change is needed.

## Builtins

- ✅ `awk [-F SEP] [-v NAME=VALUE] ['PROGRAM'|-f PROGRAM-FILE] [FILE]...` — practical POSIX-oriented text processing with BEGIN/main/range/END rules, fields, scalars, associative arrays, POSIX-oriented regex, control flow, user functions, `print`/`printf`, and common string builtins. Input files honor `AllowedPaths`; evaluated expressions, strings, records, rules, statements, loop iterations, function calls/depth, regex work, substitution metadata, and stdout are bounded. awk programs cannot execute commands: `system()`, every form of `getline`, `close()`, and command pipes are rejected, as are file-output redirection and GNU-only features such as `gensub`, `asort`/`asorti`, `strtonum`, `IGNORECASE`, the third `match` argument, GNU boundary escapes, malformed-UTF-8 byte matching, and nondecimal source literals. Exact cross-implementation `printf`/numeric edge compatibility, including NaN/infinity spellings and uncommon flag combinations, is also outside the profile. Run `awk --help` for the exact profile.
- ✅ `break` — exit the innermost `for` loop
- ✅ `cat [-AbeEnstTuv] [FILE]...` — concatenate files to stdout; supports line numbering, blank squeezing, and non-printing character display
- ✅ `cd [-LP] [-|DIRECTORY]` — change the shell's working directory; targets must lie inside `AllowedPaths`; with no operand changes to `$HOME`, with `-` swaps to `$OLDPWD` (printing the new path); `-L` (default) preserves symlinks, `-P` resolves them; `-e`, `-@`, `CDPATH`, `~user` not supported
- ✅ `continue` — skip to the next iteration of the innermost `for` loop
- ✅ `cut [-b LIST|-c LIST|-f LIST] [-d DELIM] [-s] [-n] [--complement] [--output-delimiter=STRING] [FILE]...` — remove sections from each line of files
- ✅ `df [-hHkPTialx] [-t TYPE] [-x TYPE] [--total] [--no-sync]` — report file system disk space usage (Linux/macOS only; on Windows `df` exits 1 with `df: not supported on this platform` because mount enumeration goes through `/proc/self/mountinfo` on Linux and `getfsstat(2)` on macOS, neither of which has a Windows equivalent); Linux reads `/proc/self/mountinfo` directly via `os.Open`, bypassing `AllowedPaths`; positional `FILE` operands and `--sync`, `-B`, `--output` are not supported; mount table capped at 100 000 entries
- ✅ `du [-asScSLP0bhkm] [-d N] [--apparent-size|--si] [FILE]...` — estimate file space usage; recursion capped at depth 256 and hardlink-dedup tracking capped at 2²⁰ entries; `--files0-from`, `--exclude-from`/`-X`, `--exclude` are rejected (data-exfiltration / file-driven control); `-B`/`--block-size`, `-t`/`--threshold`, `-x`/`--one-file-system`, `--inodes`, `--time`, `-l`/`--count-links` are not implemented
- ✅ `echo [-neE] [ARG]...` — write arguments to stdout; `-n` suppresses trailing newline, `-e` enables backslash escapes, `-E` disables them (default)
- ✅ `exit [N]` — exit the shell with status N (default 0)
- ✅ `false` — return exit code 1
- ✅ `find [-L] [-P] [PATH...] [EXPRESSION]` — search for files in a directory hierarchy; supports `--help`, `-name`, `-iname`, `-path`, `-ipath`, `-type` (b,c,d,f,l,p,s), `-size`, `-empty`, `-newer`, `-mtime`, `-mmin`, `-perm`, `-maxdepth`, `-mindepth`, `-print`, `-print0`, `-exec CMD {} \;`, `-execdir CMD {} \;`, `-prune`, `-quit`, logical operators (`!`, `-a`, `-o`, `()`); blocks `-delete`, `-regex` for sandbox safety
- ✅ `free [-h]` — report host memory and swap usage; a narrow read-only investigation snapshot, not a remediation or repeated-sampling command (see planned `vmstat`); Linux only, reads `/proc/meminfo` directly via `os.Open`, bypassing `AllowedPaths` (on macOS/Windows `free` exits 1 with `free: not supported on this platform` because neither exposes the buffers/cache/shared breakdown through a syscall this shell can call without cgo); `-h`/`--human` prints IEC binary sizes (e.g. `1.5Gi`); `-b`/`-k`/`-m`/`-g`/`--si`/`-w`/`-t`/`-s`/`-c` (repeated sampling) are not supported
- ✅ `grep [-EFGivclLnHhoqsxw] [-e PATTERN] [-m NUM] [-A NUM] [-B NUM] [-C NUM] PATTERN [FILE]...` — print lines that match patterns; uses RE2 regex engine (linear-time, no backtracking)
- ✅ `head [-n N|-c N] [-q|-v] [FILE]...` — output the first part of files (default: first 10 lines); `-z`/`--zero-terminated` and `--follow` are rejected
- ✅ `help [--all] [feature|command]` — display rshell features, a concise unsupported-feature summary, commands available in the current mode, allowlisted commands that require remediation mode, commands disabled by the `AllowedCommands` policy, the configured `AllowedPaths` sandbox roots grouped by read-only and read-write access, and the effective `AllowedSystemServices` unit/action grants in `UNIT:ACTION[+ACTION...]` form (with explicit default-deny notices when either policy is empty); remediation-required commands always include descriptions, while `--all` adds descriptions to commands disabled by policy; in read-only mode configured non-read grants remain visible but are marked inactive; with a topic, show detailed help for that feature or command
- ✅ `ip [-o|-4|-6|--brief] addr|link [show] [dev IFNAME]` — show network interface addresses and link-layer info (read-only); write ops (`add`, `del`, `flush`, `set`), namespace ops (`netns`, `-n`), and batch mode (`-b`/`-B`/`--force`) are blocked
- ✅ `ip route [show|list]` — show IPv4 routing table (Linux only; reads `/proc/net/route` directly via `os.Open`, bypassing `AllowedPaths`); at most 10 000 entries loaded; lines longer than 1 MiB abort parsing with an error (exit 1)
- ✅ `ip route get ADDRESS` — show the route selected by longest-prefix-match for ADDRESS (Linux only); write ops (`add`, `del`, `flush`, `replace`, `change`, `save`, `restore`) are blocked; `-6` (IPv6 routing) is not supported
- ✅ `journalctl (-u SERVICE...|-k) [-b] [-n COUNT] [--since TIME] [-o short|cat]` — bounded journal investigation for exact system services or current-boot kernel messages; defaults to 100 and caps at 1,000 entries, 32 service scopes, and 64 KiB per selected field; `short` sanitizes controls, non-graphic Unicode, and invalid UTF-8 and labels unidentified kernel entries `kernel`, while `cat` matches host `journalctl` within the field bound by writing each `MESSAGE` value unchanged and appending one newline; service reads require exact `SERVICE:read` grants and kernel reads require `systemd-journald.service:read`; also supports `--disk-usage` with `systemd-journald.service:read`, plus remediation-only `--rotate`, `--vacuum-size SIZE`, `--vacuum-time AGE`, and `--dry-run` with `systemd-journald.service:clean`; time-only vacuum removes every eligible archive at least `AGE` old, while size vacuum targets the total allocated journal storage reported by `--disk-usage` (active plus archived, matching host `journalctl --vacuum-size`), requires `AGE` as a minimum deletion age, never deletes active journals or newer archives merely to reach its target, and reports the remaining allocated bytes when the target is still exceeded; rotation may precede vacuuming, while `--dry-run` cannot rotate; bare/unrestricted reads, globbed services, arbitrary matches, follow mode, raw structured output, alternate sources, cursors, machines, namespaces, and arbitrary boot selection are unavailable; log reads use a bounded pure-Go parser for regular/compact and XZ/LZ4/Zstandard journal files with no host `journalctl`, cgo, or libsystemd dependency; rotation requires Linux with procfs descriptor links at `/proc/self/fd`, and mounted-file usage/vacuum operations support Linux/macOS
- ✅ `jq [-hcernsR] [--arg NAME VALUE] [--argjson NAME JSON] [FILTER] [FILE]...` — self-contained jq 1.8 subset supporting fields, indexes, iteration, `?`, pipes, generators, literals, constructors, variables, comparisons, boolean operators, `//`, arithmetic, and `select`, `map`, `length`, `keys`, `has`, `type`, `not`, and `empty`; an omitted filter is identity. Up to 64 operands are accepted as identity-verified regular files through `AllowedPaths`; special files and descriptor portals are rejected. Limits include 64 KiB/4,096 nodes per filter, 8 MiB cumulative input, 1 MiB raw lines, 64-level/65,536-node/4-MiB values, 100,000 evaluation steps, 10,000 results, 131,072 retained nodes/8 MiB of evaluator state, 1 MiB output, and 64 variable bindings/1 MiB of binding text. Evaluation budgets are cumulative and fail hard. Assignments, user-defined functions, reduce/foreach, try/catch, regex, recursion, modules, environment access, file-based filters/variables, streaming, dates, formatters, `as`, conditionals, slices, array-index-by-array, and `$__loc__` are unavailable; number and per-value-limit differences are documented below
- ✅ `logrotate (-s SIZE|-f) [-n] [-v] FILE...` — remediation-mode helper that truncates log files to zero bytes through `AllowedPaths`; `--size SIZE` truncates only files at least SIZE bytes, `--force` explicitly truncates without a threshold, `--dry-run` reports without modifying files, and `--verbose` reports per-file actions. Symlinked write targets are rejected with `symlinks are not supported as write targets`, and, on Unix, hard-linked targets (link count > 1) with `hard links are not supported as write targets` — not enforced on Windows, where no link count is available from an open handle; pass the real, single-linked log path instead. This is not a full `logrotate(8)` replacement: no config parsing, rename-based rotation, retained copies, compression, state files, or rotate scripts.
- ✅ `lsof [-p PIDLIST] [-c NAME] [-u UIDLIST] [-a]` — list open files, with emphasis on files still open after being deleted (Linux only; on macOS/Windows `lsof` exits 1 with `lsof: not supported on this platform`); reads `/proc/<pid>/fd/*` metadata (COMMAND/PID/USER/FD/TYPE/DEVICE/SIZE/NODE) directly via `os.Readlink`/`unix.Stat`, bypassing `AllowedPaths` like `ss`/`df`/`free`, but the resolved NAME path is checked against `AllowedPaths` and replaced with `(restricted)` (or `(restricted) (deleted)`) when outside every configured root: a deliberate divergence, since NAME can point anywhere on the host filesystem; `-p` selects by PID list, `-c` by command-name prefix (literal), `-u` by numeric UID list, `-a` ANDs the selectors instead of the default OR; network detail (`-i`/`-U`/`-s`/`-T`/`-n`/`-P`), directory-tree scans (`+d`/`+D`), repeat mode (`+r`/`-r`), device-cache files (`-D`/`-f`/`+f`), and login-name UID resolution are not supported
- ✅ `sort [-rnhubfds] [-k KEYDEF] [-t SEP] [-c|-C] [FILE]...` — sort lines of text files; `-h`/`--human-numeric-sort` orders by SI suffix (none < K/k < M < G < T < P < E < Z < Y < R < Q) then by numeric value (single-letter suffixes only — `Ki`, `Mi`, etc. are not recognised); `-o`, `--compress-program`, and `-T` are rejected (filesystem write / exec)
- ✅ `ss [-tuaxlans4689Hoehs] [OPTION]...` — display network socket statistics; reads kernel socket state directly via `os.Open` (bypassing `AllowedPaths`) from: Linux: `/proc/net/`; macOS: sysctl; Windows: iphlpapi.dll; `-F`/`--filter` (GTFOBins file-read), `-p`/`--processes` (PID disclosure), `-K`/`--kill`, `-E`/`--events`, and `-N`/`--net` are rejected
- ✅ `stat -f FILE...` — report filesystem status for each operand through `AllowedPaths`: filesystem ID/type, name-length limit, block sizes, total/free/available blocks, and inode totals; supported on Linux, macOS, and Windows, with unavailable platform fields rendered honestly (`Namelen: ?` on macOS and inode totals/free as `-` on Windows); ordinary file-status mode, custom formats, terse output, and cache controls are not implemented
- ✅ `systemctl [--system] [--no-pager] COMMAND [OPTION]... [UNIT]...` — remediation-mode-only bounded Linux system-manager inspection and control for exact, fully suffixed unit names; every invocation, including `--help`, bare/list/status inspection, and mutations, fails before grant lookup or manager access in read-only mode; once enabled, a bare invocation is restricted `list-units`, which supports `--all`, `--type`, `--state`, and `--no-legend` but considers only units with exact `read` grants; by default it returns already-loaded units that are active, failed, or carrying a job, while `--all` may load valid read-granted candidates and includes inactive units (nonexistent names may be omitted); the supported command set is exactly `list-units`, `status`, `start`, `stop`, `reload`, `restart`, `enable`, and `disable`; listing and status require exact `read` grants, and each mutation requires its matching exact action grant for every operand before any effect; runtime jobs use fixed `replace` mode, process multiple anchors sequentially in operand order, and may act on dependency-related units through normal systemd transaction semantics; enable/disable may follow `[Install] Alias=`, `Also=`, and template `DefaultInstance=` into auxiliary unit-file changes, then globally reload the manager, which may run generators and pick up unrelated host changes; granted unit payloads, install metadata, aliases, and dependency graphs are operator-trusted, so dedicated lifecycle verbs being absent does not make lifecycle effects impossible through a granted unit; every manager-backend operation has a fixed 30-second cap in addition to the runner deadline; accepts `.service`, `.timer`, `.socket`, and other valid unit types, with at most 32 operands and 64 KiB per returned field; `status` omits process command lines, logs, arbitrary properties, unit-file paths, and D-Bus object paths, and every human-readable field is sanitized; `show`, the `is-*` predicates, conditional restart variants, `reset-failed`, `--now`, arbitrary enumeration/properties, globs, implicit `.service`, user/machine/root/image targets, `clean`, standalone `daemon-reload`, `kill`, unit-file editing/linking/masking/presets, dedicated power-management verbs, asynchronous jobs, arbitrary job modes, and explicit dependency-expansion switches are unavailable; requires procfs descriptor links at `/proc/self/fd` and uses the configured public system D-Bus socket without executing host `systemctl`
- ✅ `ls [-1aAdFhlpRrSt] [--offset N] [--limit N] [FILE]...` — list directory contents; `--offset`/`--limit` are non-standard pagination flags (single-directory only, silently ignored with `-R` or multiple arguments, capped at 1,000 entries per call); offset operates on filesystem order (not sorted order) for O(n) memory
- ✅ `ntfs-du [--apparent-size] [--top-files N] [--top-ext N] [-d N] [--tree-limit N] [--min SIZE] [--exclude PATH]... [--find-ext CSV|--find-glob PAT|--find-regex RE]... [--find-limit N] [--output json] [FOLDER]` — quickly find large folders, files, and file extensions across an NTFS volume by reading the raw `$MFT`, so runtime scales with the volume's file count rather than the starting folder (use `du` for a small subtree); `--find-ext`/`--find-glob`/`--find-regex` locate specific files, and `--top-ext`/`--find-ext` support extensions of any length (**Windows only** — registered only on Windows, absent elsewhere; **requires Administrator**; NTFS volumes only); opens the raw volume device `\\.\<drive>:` directly, bypassing `AllowedPaths` (same trade-off as `ss`/`df`/`ip route`) and reporting names/sizes across the whole volume regardless of sandbox roots; scans the current drive root by default (depth 1, at most 100 tree nodes, top-10 files and extensions, `--min` 100M); `--tree-limit` caps emitted tree nodes at 1,000 and `--top-ext` caps extension results at 64; emits JSON only (`--output json`; a human-readable format is planned)
- ✅ `ping [-c N] [-W DURATION] [-i DURATION] [-q] [-4|-6] [-h] HOST` — send ICMP echo requests to a network host and report round-trip statistics; `-f` (flood), `-b` (broadcast), `-s` (packet size), `-I` (interface), `-p` (pattern), and `-R` (record route) are blocked; count/wait/interval are clamped to safe ranges with a warning; multicast, unspecified (`0.0.0.0`/`::`), and broadcast addresses (IPv4 last-octet `.255`) are rejected — note: directed broadcasts on non-standard subnets (e.g. `.127` on a `/25`) are not blocked without subnet-mask knowledge
- ✅ `pmap [-x] [-h|--help] PID...` — report per-process virtual memory mappings (start address, size, permission mode, mapping label); Linux reads `<ProcPath>/<pid>/maps` (or `smaps` for `-x`) directly via `os.Open`, bypassing `AllowedPaths` (the configured proc root is trusted and the remaining path is derived only from the numeric PID); Windows enumerates committed regions via `VirtualQueryEx` and labels each by its `MEMORY_BASIC_INFORMATION` type rather than a resolved file path (`-x` exits 1 with `pmap: -x is not supported on this platform`, since Windows has no per-region RSS/Dirty breakdown without a working-set walk); macOS enumerates regions via the `proc_pidinfo(PROC_PIDREGIONINFO)` kernel call reached through the raw `syscall.SYS_PROC_INFO` trap (`-x` also exits 1 with `pmap: -x is not supported on this platform`, since `proc_regioninfo` reports whole-shadow-chain resident/dirty counts rather than the private Rss/Dirty split pmap's extended columns expect); the process header shows only the comm/executable name, never argv; full-path, device, range, raw-kernel-name, rc-file read/write, extra-extended, quiet, and version flags are not supported
- ✅ `ps [-e|-A] [-f] [-p PIDLIST] [-o FORMAT] [--sort SPEC]` — report process status; default shows current-session processes; `-e`/`-A` shows all; `-f` adds UID/PPID/STIME columns; `-p` selects by PID list; repeatable `-o`/`--format` accepts the canonical fields `pid`, `ppid`, `uid`, `state`, `tty`, `stime`, `time`, `comm`, `rss`, `vsz`, `pmem`, `pcpu`, and `etime`, with `%cpu` and `%mem` as aliases for `pcpu` and `pmem`; `--sort` accepts the same fields and aliases in a comma- or space-separated list with optional `+` (ascending) or `-` (descending) prefixes; `pcpu` is the lifetime average since process start, not an interval measurement; unavailable requested metrics render as `-`; process names are comm-only, argv/environment/executable-path selectors are rejected, and process argv and environment are never read; on macOS, `stime`/`etime` remain available while `rss`/`vsz`/`pmem`/`time`/`pcpu` may be unavailable for processes the caller cannot inspect; on Windows, `rss`/`vsz` use the compatibility-sensitive `SystemProcessInformation` snapshot API and may be unavailable if it fails (`pmem` is then unavailable too)
- ✅ `printf FORMAT [ARGUMENT]...` — format and print data to stdout; supports `%s`, `%b`, `%c`, `%d`, `%i`, `%o`, `%u`, `%x`, `%X`, `%e`, `%E`, `%f`, `%F`, `%g`, `%G`, `%%`; format reuse for excess arguments; `%n` rejected (security risk); `-v` rejected
- ✅ `pwd [-LP]` — print the absolute pathname of the current working directory; `-L` (default) prints the shell's tracked logical path, `-P` resolves all symlinks; `-P` is best-effort within the sandbox (path components above `AllowedPaths` pass through unresolved); `--version` rejected
- ✅ `read [-r] [-p PROMPT] [-d DELIM] [-n N] [-N N] [-t SECS] [NAME...]` — read one delimited chunk from stdin and assign each IFS-split field to a shell variable (defaulting to `REPLY` when no NAME is given); `-n`/`-N` are capped at 1 MiB; non-raw mode treats `\<newline>` as a line continuation (both characters are dropped) and `\<X>` for any other `X` (including the active custom delimiter under `-d`) as a literal `X` with the backslash removed — e.g. `printf 'a\,b,c' | read -d , x` assigns `x="a,b"`; `-p` is suppressed unless stdin is a terminal (matches bash); `-a` (array), `-s` (silent), `-u` (read from FD), `-e` (readline), and `-i` (initial text) are not implemented
- ✅ `rm [-v] FILE...` — remove files; **remediation mode only**, targets must be within a `:rw` `AllowedPaths` root; directories are always rejected, even empty ones (there is no recursive or `-d`/`--dir` mode) — any other non-directory entry (regular file, symlink, FIFO, socket, device node) may be removed; a symlink argument removes the link itself, never its referent; a hard link is removable (unlinking one name leaves the inode and its other names intact), unlike the write builtins which reject hard-linked targets; at most 10 files per invocation, checked before any file is removed, **and** at most 100 files in total per script run — the run-wide budget is shared across every `rm` invocation, loop iteration, subshell, and pipeline stage, so `for f in *; do rm "$f"; done` and `find … | xargs -n1 rm` are bounded too; only successful removals are charged, and each new run starts with a fresh budget; `-v`/`--verbose` prints `removed 'FILE'` per file; `-r`/`-R`/`--recursive`, `-f`/`--force`, `-i`/`-I`/`--interactive`, `-d`/`--dir`, `--preserve-root`, `--no-preserve-root`, and `--one-file-system` are rejected as unknown flags
- ✅ `sed [-n] [-e SCRIPT] [-E|-r] [-i] [SCRIPT] [FILE]...` — stream editor for filtering and transforming text; uses the RE2 regex engine with leftmost-longest matching; `-f` rejected; `e`/`w`/`W`/`r`/`R` commands blocked. `-i`/`--in-place` is remediation-mode-only (refused with `in-place editing requires remediation mode` otherwise) and edits each file operand through `AllowedPaths`: backup-suffix forms are unsupported since there is no atomic rename primitive to create the backup: an explicit `=value` form (`-i=.bak`, `--in-place=.bak`) is rejected with an error, and a suffix attached directly to the cluster whose characters are not themselves valid short flags (e.g. `-i.bak`, since `.` is not a registered shorthand) is also rejected, by pflag itself, with `invalid option -- '.'` — the edit does not happen and the file is left untouched in that case. Only when every character of the attached suffix happens to also be a valid *boolean* short flag (e.g. `-iE`, `-niE`) does the whole token instead parse per pflag's standard getopt-cluster semantics as `-i` followed by more combined single-character flags (`-iE` enables `-i` and `-E`, silently discarding any backup-suffix intent rather than rejecting it, and the edit *does* happen in that case, in place, exactly as bare `-i` would) — a deliberate, documented divergence from GNU sed's `-i[SUFFIX]`, since this shell never creates the backup file either way. This does NOT hold once the cluster reaches `-e` (the one sed short flag that takes a value): pflag stops there and consumes everything after it in the same token — or, if nothing follows in that token, the entire next argument — as `-e`'s expression value instead of parsing it as further combined flags, per pflag's own getopt-cluster semantics for value-taking short flags in general, not something sed itself special-cases. `sed -ien 's/a/b/' f`, for example, does NOT parse as `-i`, `-e`, and `-n`: it parses as `-i` (enabled) plus `-e n` (script text literally "n"), leaving `s/a/b/` and `f` as file operands instead of the intended script — verified against this shell's actual pflag-based parser, not GNU sed, which has its own, different short-option parser entirely; `-` (stdin) is rejected as a target; a missing file is an error rather than being created; each file is treated as a separate stream matching GNU sed `-s` (line numbers, `$`, and the hold space all reset per file; the last-used regex for an empty `//` pattern persists across files; `h`/`H`/`g`/`G`/`x` carry the moved content's own newline-termination state along with it, not the destination's prior state); unlike the streaming default mode (which intentionally always terminates output with `\n`), `-i` reproduces GNU sed's on-disk behaviour exactly, including a source file whose last line has no trailing newline (except `q`'s implicit print, which — unlike ordinary end-of-file auto-print — always terminates with `\n` even when it fires on that unterminated final line, matching GNU sed 4.9's own divergence between the two, and — unlike `Q`, which never does — `q` also flushes any newline an earlier explicit print already deferred under `-n`, even though `-n` suppresses `q`'s own implicit print entirely); the original file is read fully into memory up front (capped at 256 MiB) rather than consumed incrementally; files already oversized at stat time are rejected before reading, and a bounded read still detects subsequent growth without storing the extra probe byte. Both original and rewritten buffers have capacity capped at 256 MiB each (transient growth copies and engine state add overhead), so a script that quits early (`q`/`Q`) still has the complete original available for a restore, and an earlier file's failure is not discarded by a later file's `q`; the destructive write is pinned to the exact file that was read (rejecting a path swapped for a different, otherwise-acceptable regular file since the read) and its target-type validation goes through a single-file-descriptor primitive (open non-blocking, fstat to confirm a regular file, write, truncate) so a FIFO/device swapped in for the target is rejected before the write-open can ever block; if a completed write fails partway (e.g. disk full), a best-effort restore writes the original through the same primitive. Each write call has a combined acquisition/write/outcome-resolution budget of at most 30 seconds, shortened by the caller's deadline; up to one second (at most half the remaining budget) is reserved for resolving a writer after its earlier write deadline. Explicit cancellation stops the wait. A writer still unresolved at the end of that budget may have partially overwritten the file: restoration is skipped to avoid a concurrent write, and remaining operands are not processed. A resolved partial-write failure can instead trigger restoration with its own independent, at-most-30-second cleanup budget; if that best-effort restore's own outcome is left unresolved (its bounded wait for a still-in-progress background write timed out), the whole invocation stops processing any remaining file operands instead of continuing on as it would for an ordinary per-file failure, since a later operand naming the same path could otherwise read and overwrite the same inode a background write may still be mutating; `q`/`Q` still commit whatever was produced up to the quit point and then stop processing any remaining files
- ✅ `sha256sum [-c|--check] [--quiet] [--status] [FILE]...` — stream SHA-256 digests for identity-verified regular files or standard input, or verify GNU-style and tagged SHA256 checksum manifests; file contents are hashed as raw bytes on every platform; manifests accept GNU horizontal whitespace plus LF, CRLF, and CR record endings; `--quiet` and `--status` apply only with `--check`; manifests are capped at 64 KiB per line, 8 MiB of raw input, and 1,024 valid entries, dynamic check output is capped at 1 MiB, and at most 1,024 operands are accepted per invocation; file and manifest operands must be regular files within `AllowedPaths`, so directories, devices, FIFOs, sockets, and descriptor portals are rejected without blocking; Windows alternate-data-stream syntax is not specially interpreted and remains subject to the same sandbox and regular-file identity checks; `--binary`, `--text`, `--tag`, `--zero`, `--ignore-missing`, `--strict`, `--warn`, and `--version` are not supported
- ✅ `strings [-a] [-n MIN] [-t o|d|x] [-o] [-f] [-s SEP] [FILE]...` — print printable character sequences in files (default min length 4); offsets via `-t`/`-o`; filename prefix via `-f`; custom separator via `-s`
- ✅ `tail [-n N|-c N] [-q|-v] [-z] [FILE]...` — output the last part of files (default: last 10 lines); supports `+N` offset mode; `-f`/`--follow` is rejected
- ✅ `tee [-a] [FILE]...` — copy standard input to standard output and to each FILE; **remediation mode only**, every FILE target must be within a `:rw` `AllowedPaths` root; `-a`/`--append` appends instead of overwriting; a `-` FILE operand is opened as a literal file named `-` through the sandbox, matching current GNU tee (the historical stdout-alias special case was removed in coreutils 8.24); a failing destination (sandbox denial, non-regular target, hard link) is reported and skipped while output continues to standard output and every remaining FILE, matching GNU tee; hard-linked write targets are rejected with `hard links are not supported as write targets` on Unix, and every non-regular write target (FIFO, socket, device) is rejected via a non-blocking pre-open check so a readerless FIFO cannot hang the shell; at most 64 FILE operands are accepted (each is held open concurrently for the whole copy, unlike jq/sha256sum's one-at-a-time file processing, so the cap stays well under common default descriptor limits such as macOS's 256), checked before any destination is opened, so a glob or repeated operand cannot exhaust the embedding process's descriptor table; the stdin copy loop stops as soon as every destination (including standard output) has stopped accepting writes, rather than draining a long-lived or infinite source once every consumer is gone; `-i`/`--ignore-interrupts` and `-p`/`--output-error` are rejected as unknown flags
- ✅ `test EXPRESSION` / `[ EXPRESSION ]` — evaluate conditional expression (file tests, string/integer comparison, logical operators)
- ✅ `tr [-cdsCt] SET1 [SET2]` — translate, squeeze, and/or delete characters from stdin
- ✅ `truncate -s SIZE [-c] [FILE]...` — shrink or extend file size; **remediation mode only**, target must be within a `:rw` `AllowedPaths` root; SIZE supports GNU suffix grammar (K/k/KiB/kiB=1024, KB/kB=1000, M/G/T similarly, P/E uppercase-only); relative-size modifiers and `--reference`/`--io-blocks` are rejected; hard-linked targets (link count > 1) are rejected with `hard links are not supported as write targets` on Unix
- ✅ `true` — return exit code 0
- ✅ `uname [-asnrvm]` — print system information (Linux only; reads from `/proc/sys/kernel/`, respects `--proc-path`)
- ✅ `uniq [OPTION]... [INPUT]` — report or omit repeated lines
- ✅ `uptime [-ps]` — tell how long the system has been running; shows current time, uptime duration, and load averages (1/5/15 min); user count omitted (privacy); `-p` prints human-readable duration; `-s` prints boot time as `YYYY-MM-DD HH:MM:SS`; load average unavailable on Windows
- ✅ `vmstat [-a] [-w] [-S k|K|m|M] [-s] [delay count]` — report virtual memory, swap, IO, and CPU pressure statistics; no arguments prints a since-boot snapshot, positive `delay count` samples for at most 29 seconds of total wait time, and `-s`/`--stats` prints a full counter summary; `-S` scales memory and swap-rate columns; reads `/proc/*` (Linux) or `sysctl(3)` (macOS) directly, bypassing `AllowedPaths` (same exception as `df`/`ss`/`ip route`); macOS lacks several counters (Mach-only), shown as `-` rather than fabricated; `-d`, `-p`, `-f`, `-m`, `-t`, `-n`, `-V` are not implemented; not supported on Windows
- ✅ `wc [-l] [-w] [-c] [-m] [-L] [FILE]...` — count lines, words, bytes, characters, or max line length
- ✅ `xargs [-0] [-a FILE] [-d DELIM] [-E EOF-STR] [-I REPLSTR] [-L N] [-n N] [-r] [-s N] [-t] [-x] [COMMAND [INITIAL-ARGS]...]` — build and execute commands from standard input; only invokes other registered builtins (subject to `CommandAllowed`), so the GTFOBins shell-escape `xargs … /bin/sh` is rejected; flags outside the supported set above (e.g. `-p` interactive, `-P` parallel, `-o`/`--open-tty`, `--show-limits`) are rejected as unknown
- ❌ All other commands — rejected with exit code 127; the public API does not expose an external-command executor

## Variables

- ✅ Assignment: `VAR=value`
- ✅ Expansion: `$VAR`, `${VAR}`
- ✅ `$?` — last exit code (the only supported special variable)
- ✅ Inline assignment: `VAR=value command` (scoped to that command)
- ✅ Command substitution: `$(cmd)`, `` `cmd` `` — captures stdout; trailing newlines stripped; `$(<file)` shortcut reads file directly (gated on `cat` being in the command allowlist); output capped at 1 MiB
- ❌ Arithmetic expansion: `$(( expr ))`
- ❌ Array assignment: `arr=(a b c)`, `arr[0]=x`
- ❌ Append assignment: `VAR+=value`
- ❌ Parameter expansion operations: `${#var}`, `${var:-default}`, `${var:=default}`, `${var:?msg}`, `${var:+alt}`, `${var:offset}`, `${var/pattern/repl}`, `${var#prefix}`, `${var%suffix}`, `${!var}`, `${!prefix*}`, case conversion
- ❌ Positional parameters: `$1`–`$9`, `$@`, `$*`, `$#`, `$0`
- ❌ Special variables: `$!`, `$LINENO`

## Control Flow

- ✅ `for VAR in WORDS; do CMDS; done`
- ✅ `while CONDITION; do CMDS; done` — runs CMDS while the last command of CONDITION exits 0
- ✅ `until CONDITION; do CMDS; done` — runs CMDS while the last command of CONDITION exits non-zero
- ✅ `&&` — AND list (short-circuit)
- ✅ `||` — OR list (short-circuit)
- ✅ `!` — negation (inverts exit code)
- ✅ `{ CMDS; }` — brace group
- ✅ `;` and newline as command separators
- ✅ `if` / `elif` / `else`
- ✅ Subshells: `( CMDS )` — runs commands in an isolated child environment; variable changes do not propagate to the parent; exit does not terminate the parent
- ❌ `case`
- ❌ `select`
- ❌ C-style for loop: `for (( i=0; i<N; i++ ))`
- ❌ Functions: `fname() { ... }`

## Pipes and Redirections

### Always supported (both modes)

- ✅ `|` — pipe stdout
- ✅ `<` — input redirection (read-only, within AllowedPaths)
- ✅ `<<DELIM` — heredoc
- ✅ `<<-DELIM` — heredoc with tab stripping
- ✅ `2>&1`, `>&2` — file descriptor duplication between stdout (1) and stderr (2)
- ✅ `> FILE`, `>> FILE` — write/truncate or append to a regular file; **remediation mode only**, target must be within a `:rw` `AllowedPaths` root (exit 1 otherwise)
- ✅ `2> FILE`, `2>> FILE` — same rules applied to the stderr stream; **remediation mode only**
- ✅ `&> FILE`, `&>> FILE` — redirect both stdout and stderr to the same file; **remediation mode only**, same `AllowedPaths` enforcement
- ❌ `|&` — pipe stdout and stderr (bash extension)
- ❌ `<<<` — herestring (bash extension)
- ❌ `<>` — read-write open (blocked in all modes)
- ❌ `<&N` — input file descriptor duplication

For a simple command, rshell expands only enough words to identify the command, then applies `AllowedCommands`, selective-elevation, and remediation-mode policy before expanding the remaining arguments, inline assignments, or redirects. A rejected command therefore cannot run command substitutions from unused arguments, read or expand a heredoc, or read, create, append to, or truncate a redirect target. Redirect-only statements remain authorized by the applicable mode and `AllowedPaths` policy. After authorization, redirects follow normal shell timing: they are established before command execution, so an authorized command that later exits nonzero can still create or modify its redirect targets.

Redirect targets undergo brace, tilde, parameter, command-substitution, field-splitting, and pathname expansion where those features are permitted. Exactly one field must result; zero or multiple fields fail with `ambiguous redirect` before opening that target. Quotes preserve literal whitespace and glob characters. The `$(<file)` shortcut uses the same expansion rules. Globs only enumerate sandbox-accessible paths, and the expanded target remains subject to `AllowedPaths` and mode checks.

### Output redirections (mode-dependent)

| Redirect | read-only mode | remediation mode |
|----------|---------------|-----------------|
| `>/dev/null`, `2>/dev/null`, `&>/dev/null` | ✅ always accepted (discards output) | ✅ always accepted |
| `>>/dev/null`, `&>>/dev/null` | ✅ always accepted (same effect as truncate) | ✅ always accepted |
| `> FILE`, `>| FILE` | ❌ exit 2 (parse-time rejection) | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |
| `>> FILE` | ❌ exit 2 | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |
| `2> FILE` | ❌ exit 2 | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |
| `2>> FILE` | ❌ exit 2 | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |
| `&> FILE` | ❌ exit 2 | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |
| `&>> FILE` | ❌ exit 2 | ✅ within `:rw` `AllowedPaths`; exit 1 outside or in read-only roots |

The `exit 2` rejections above apply to a **literal** target, which the validator
can resolve before the script runs. A target that is not a plain literal —
`> "$F"`, `> $F`, `> "/dev/null"` — is not resolved at validation time (the
validator never expands user input); it is checked at run time against the
expanded value instead. In read-only mode the policy is identical (only
`/dev/null` is accepted), but the failure mode is gentler: that single command
fails with exit 1 and the script continues, rather than the whole program being
rejected with exit 2. So `F=/dev/null; echo x > "$F"` succeeds, and
`F=out.txt; echo x > "$F"` prints
`> out.txt: file redirection is only supported for /dev/null` and fails just
that command. A command substitution in the target (`> $(cmd)`) is the one
dynamic form still rejected with exit 2, so that a blocked redirect never runs
the substituted command.

## Quoting and Expansion

- ✅ Single quotes: `'literal'`
- ✅ Double quotes: `"with $expansion"`
- ✅ Globbing: `*`, `?`, `[abc]`, `[a-z]`, `[!a]`
- ✅ Bounded expansion: at most 16,384 expanded arguments and 10 MiB of expanded argument text per command; at most 64 MiB of expanded argument, assignment, redirect, and heredoc text per `Run`, shared across loops, subshells, and pipeline stages; the execution context is checked between produced fields
- ✅ Line continuation: `\` at end of line
- ✅ Comments: `# text`
- ❌ Extended globbing: `@(pat)`, `*(pat)`, etc.
- ❌ Tilde expansion: `~`, `~/path`, `~user`
- ❌ Process substitution: `<(cmd)`, `>(cmd)`

## Execution

- ✅ AllowedCommands — allowlists names in the `rshell:` namespace (e.g. `rshell:cat`); registered commands may execute, while allowlisted unknown names still fail with exit code 127; if not set, no commands are allowed
- ✅ AllowedSystemServices policy — one shared default-deny capability map for exact unit names with `read`, `clean`, `start`, `stop`, `reload`, `restart`, `enable`, and `disable` actions; the `*` action wildcard grants every action supported by the running and future rshell versions for its exact unit and is expanded in `help`; actionless grants are ignored, invalid names/actions are skipped, names are case-sensitive, are not normalized, and cannot contain `:` or globs; all valid unit types are accepted; `read` remains usable by bounded `journalctl` queries in read-only mode, every non-read action requires remediation mode, and the entire `systemctl` builtin separately requires remediation mode while `list-units` sees only exact `read` grants; allowing all commands does not bypass this policy; a `read` grant on a `.slice` unit is the one selector whose journal results are not bounded by the granted name, because `journalctl -u SLICE` matches `_SYSTEMD_SLICE` and therefore returns every entry logged by every unit in that slice (see the trusted systemd target exception in `docs/RULES.md`); configure grants through `interp.AllowedSystemServices` or CLI `--allowed-services UNIT:ACTION[+ACTION...]`
- ✅ SystemdTargetConfig — systemd-aware builtins use standard local paths by default; explicit `JournalDirs`, `MachineIDPath`, `JournalControlSocket`, and `ManagerBusSocket` paths support container mount layouts without falling back to local endpoints; target paths are trusted configuration and bypass `AllowedPaths`; explicit mounts must all refer to the same host; manager operations use the public system D-Bus socket (default `/run/dbus/system_bus_socket`), pin its inode, authenticate, and verify the systemd manager peer's machine ID, while the private `/run/systemd/private` endpoint is unsupported
- ✅ AllowedPaths filesystem sandboxing — restricts all file access (read and write) to specified directories. Entries may end with `:ro` or `:rw` to indicate read-only and read-write permissions, respectively; entries without a suffix default to read-only. In remediation mode, write operations are accepted only inside the most-specific matching `:rw` root. Cross-root symlink fallback is read-only to avoid TOCTOU on writes; on Unix, symlink components in write targets are rejected with `symlinks are not supported as write targets` via a no-follow `openat` walk. Containment is path-based and cannot see hard links, so on Unix every content-mutating write target is additionally `fstat`ed after open and rejected with `hard links are not supported as write targets` when its link count exceeds one; `rm` is deliberately exempt (unlinking one of several names changes no content). Not enforced on Windows, where no link count is available from an open handle
- ✅ Whole-run execution timeout — callers can bound a `Run()` call via `context.Context`, `interp.MaxExecutionTime`, or the CLI `--timeout` flag; the deadline applies to the whole script. Cancellation is checked between bounded operations; a caller-owned stream blocked in `Read` or `Write` can delay return because rshell does not close borrowed streams
- ✅ ProcPath — overrides the proc filesystem path used by `ps` (default `/proc`; Linux-only; useful for testing/container environments); `ps` reads only bounded process metadata and counters, never `/proc/<pid>/cmdline` or process environment data
- ✅ RemediationMode — opt-in mode (`interp.WithMode(interp.ModeRemediation)` / `--mode remediation`) that enables file-target output redirections (`>`, `>>`, `2>`, `&>`, `&>>`) within `:rw` `AllowedPaths` and remediation-only builtins such as `truncate`, `tee`, `logrotate`, `rm`, and the entire restricted `systemctl` surface; targets outside the allowlist or inside read-only roots fail with `permission denied` (exit 1); symlinked write targets fail with `symlinks are not supported as write targets`; hard-linked write targets fail with `hard links are not supported as write targets` on Unix; `/dev/null` always accepted; `<>` remains blocked
- ✅ Privileged helper isolation (Linux) — authenticated `runCommand` and `runRemediationCommand` Private Action Runner dispatches run in fresh same-binary workers with a Landlock ABI 3+ policy derived from the verified effective `AllowedPaths`, `AllowedCommands`, and `AllowedSystemServices`, followed by a TSYNC seccomp denylist; authorized `journalctl` and remediation-mode `systemctl` calls receive only the fixed local systemd paths their grants require; `runCommand` forces ordinary path grants to read-only and never grants journal deletion rights, verified roots are pinned before execution drops back to the service account, sandbox setup fails closed, external execution is denied, and only explicitly authorized commands use controlled `setresuid` elevation
- ❌ External commands — unavailable through the stock runner because it exposes no external-command executor; allowlisting an unknown name does not make it executable
- ❌ Background execution: `cmd &`
- ❌ Coprocesses: `coproc`
- ❌ `time`
- ❌ `[[ ... ]]` extended test expressions (bash extension)
- ❌ `(( ... ))` arithmetic commands
- ❌ `declare`, `export`, `local`, `readonly`, `let`

## Environment

- ✅ Empty by default — no parent environment variables are inherited
- ✅ Caller-provided variables via the `Env` option
- ✅ `IFS` is set to space/tab/newline by default
- ✅ `ALLOWED_PATHS` — when `AllowedPaths` is configured, set to a `filepath.ListSeparator`-delimited list of resolved allowed directories (`:` on Unix, `;` on Windows)
- ❌ No automatic inheritance from the host process
- ❌ `export`, `readonly` are blocked

## Intentional Divergences

- **Time reference for `find -mmin`/`-mtime` and `ls -l`**: rshell captures `time.Now()` once at the start of each `Run()` call and shares it across all builtins in that run. Bash evaluates each command against its own invocation time. In practice this only matters for long-running scripts (e.g. `sleep 61; find . -mmin -1`) where the reference time drifts from the actual command start. Short-lived AI agent scripts are unaffected.
- **`jq` number model**: integer literals and integer-only `+`, `-`, `*`, `%`, and exact division remain exact up to 256 bits; larger values are reported and skipped. Decimal/exponent literals, mixed arithmetic, and non-integral division use `float64`, so integer arithmetic does not preserve signed zero. The integer bound prevents unbounded `big.Int` growth under chained multiplication.
- **`jq` floating-point rendering**: decimal/exponent values are stored as `float64`, so output is normalized instead of preserving source spelling. Overflow saturates to `±1.7976931348623157e+308`; underflow becomes `0`.
- **Strict JSON input in `jq`**: leading-zero numbers and raw NUL in strings are rejected instead of reproducing jq's permissive or chunk-dependent behavior. An unpaired low surrogate (`"\udc00"`) decodes to U+FFFD; an unpaired high surrogate is an error.

## Appendix

Formatting: In each category, supported features should be listed first, and the most useful ones first.
