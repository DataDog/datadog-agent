# Builtin Command Testing Rules

_[Preamble to be added]_

---

## Implementation Choices

### Flag Parsing

All flag parsing MUST use `github.com/spf13/pflag` (already a dependency in `go.mod`).
Do NOT write manual flag-parsing loops.

pflag handles POSIX/GNU conventions automatically:
- Long forms: `--lines=N` and `--lines N`
- Short compact forms: `-n3` (value glued to flag)
- Combined boolean short flags: `-qv`
- `--` end-of-flags separator

**Setup pattern:**
```go
fs := pflag.NewFlagSet("cmdname", pflag.ContinueOnError)
fs.SetOutput(io.Discard) // suppress pflag's own error output; format errors yourself
```

**Flags that need `+N` offset support** (e.g. `tail -n +5`): register as `StringP`, then
post-process the value with a helper that detects the `+` prefix.

**Error handling:** pflag parse errors (unknown flag, missing argument) are caught
by the shared wrapper in `builtins/builtins.go`, which rewrites them to the GNU
getopt wording (`unrecognized option '--foo'`, `invalid option -- 'X'`,
`option '--foo' doesn't allow an argument`, `option '--foo' requires an argument`)
and appends a `Try 'cmdname --help' for more information.` line. The command
exits 1 — a bad flag fails the command but does not abort the script.

### Supported Flags Only

Commands MUST only implement the flags listed in their supported flag set. Any flag not
explicitly registered with pflag is automatically rejected with an "unrecognized option"
error written to stderr and exit code 1. Do NOT add pre-scan loops or special-case logic
to reject specific flags by name — rely on pflag's unknown-flag handling instead.

### Help Flag

Every command MUST register `-h` / `--help` as a flag. When `--help` is passed:
- Print a usage line (`Usage: cmd [OPTION]... [FILE]...` or equivalent) to **stdout**
- Print a short description of what the command does to stdout
- Print all supported flags with brief descriptions via `fs.PrintDefaults()` to stdout
- Set exit code 0 and return

Do not write help output to stderr. Help is not an error.

### AWK Scope

The `awk` builtin is a practical POSIX-oriented text processor, not a GNU awk
compatibility layer. Keep its language surface focused on records and fields,
patterns, scalar and associative-array expressions, control flow, user
functions, `print`/`printf`, and common string builtins.

awk programs MUST NOT execute commands or open extra input streams. Reject
`system()`, every form of `getline`, `close()`, print/printf command pipes, and
file-output redirection. GNU-only builtins and variables (`gensub`,
`asort`/`asorti`, `strtonum`, `patsplit`, `IGNORECASE`, and the third `match`
argument), GNU boundary escapes, malformed-UTF-8 byte matching, and nondecimal
source literals are outside the supported profile. Exact cross-implementation
`printf` and numeric edge compatibility, including NaN/infinity spellings and
uncommon flag combinations, is also out of scope. Do not reintroduce these
features solely to match awk edge cases.

### Remediation-Only Commands

A builtin that must not run outside remediation mode MUST set
`RemediationOnly: true` on its `builtins.Command`. The interpreter enforces the
flag at dispatch, before the handler runs and therefore before flag parsing, so
`--help` is refused exactly like any other invocation. The refusal text comes
from `RemediationDeniedMessage` (defaulting to `<name>: remediation mode
required`).

Commands MUST additionally keep their own equivalent check inside the handler.
Two layers are intentional: the dispatch gate covers a builtin that forgets its
check, and the in-handler check keeps the builtin correct when called outside
the interpreter. `TestRemediationOnlyBuiltinsRefusedInReadOnlyMode` iterates
the registry, so a new remediation-only builtin is covered automatically.

When a remediation capability closure (`Remove`, `Truncate`,
`TruncateToZeroIfAtLeast`) is nil *while* `CallContext.RemediationMode` is
true, the cause is a missing writable `AllowedPaths` root, not the mode.
Commands MUST distinguish the two cases so the operator is not sent to fix the
wrong thing.

### File Access — Safe Wrappers Only

Builtins MUST access the filesystem exclusively through the narrow, sandboxed
capabilities on `CallContext`. Never call `os.Open`, `os.OpenFile`, `os.ReadFile`,
`os.ReadDir`, `os.Stat`, `os.Lstat`, or any other `os`-package filesystem function
directly.

Use `callCtx.OpenFile` for file contents, `StatFile` / `LstatFile` for file metadata,
`ReadDir` / `OpenDir` for directory entries, and `FileSystemStat` for filesystem-wide
metadata associated with a user-supplied path. These capabilities route through the
`AllowedPaths` sandbox (backed by `os.Root`) and keep access tied to rooted handles.
Bypassing them — even for a "harmless" stat or existence check — defeats the sandbox
entirely.

Use `callCtx.OpenRegularFile` for user-supplied regular files. It opens without
blocking, verifies handle identity with `os.SameFile`, and rejects special files
and descriptor portals such as `/dev/fd/N` and `/proc/self/fd/N`.

```go
// CORRECT
f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
info, err := callCtx.FileSystemStat(ctx, path)

// WRONG — bypasses the sandbox
f, err := os.Open(path)
```

Using `os` constants (`os.O_RDONLY`, `os.FileMode`) and types (`*os.File` for stdin) is fine;
only the filesystem-accessing *functions* are forbidden.

#### Trusted systemd target exception

Systemd-aware builtins MUST use the structured services on `callCtx.Systemd` and
MUST NOT open target paths themselves. The trusted `internal/systemd` backend may
read paths and connect to sockets selected by `interp.WithSystemdTarget`; those
targets intentionally bypass `AllowedPaths`, like `ProcPath`, because they are
fixed by the embedding application and cannot be supplied by shell scripts.

Configured target paths are used directly in the rshell process namespace. The
embedding application MUST ensure the journal directories, machine-ID path,
journald control socket, and manager bus socket refer to the same host. Journal
discovery and reads MUST reject symlinked machine directories and journal files
and verify file identity across open. Vacuum MUST remain rooted within each
configured journal directory, and journal control and manager bus socket access
MUST remain pinned to the validated socket inode.

Journal reads and storage metadata are limited to regular, non-symlink `.journal`
files directly under the configured machine-ID directories. The pure-Go reader
verifies each file header's machine ID, snapshots the file set and identities,
retries once after a concurrent rotation or structurally inconsistent read, and
applies fixed file, index, field-size, entry-count, decompression, and cancellation
bounds. Builtins receive selected fields only and never receive a raw journal
handle, target path, or arbitrary field-match capability.

A journal `read` grant on a `.slice` unit is the one unit selector whose result
set is not bounded by the granted name, and it MUST be documented as such. For
a non-slice unit the reader combines only bounded matches: `_SYSTEMD_UNIT=` for
the unit's own processes, `UNIT=` paired with `_PID=1` or
`_SYSTEMD_CGROUP=/init.scope` for manager messages about the unit,
`OBJECT_SYSTEMD_UNIT=` paired with `_UID=0`, and `COREDUMP_UNIT=` paired with
`_UID=0` and the fixed coredump `MESSAGE_ID=`. Each of those names the granted
unit itself, so the entries returned belong to that unit. When the requested
unit name ends in `.slice`, the reader additionally matches
`_SYSTEMD_SLICE=<unit>` with no companion requirement
(`internal/systemd/journal_query_file.go`), and journald stamps that field on
every entry produced by every unit placed in the slice. `journalctl -u
system.slice` therefore returns the log entries of every service in
`system.slice`, not the slice unit's own messages. This mirrors upstream
`journalctl -u <slice>` and MUST NOT be removed: dropping the match would make
a slice query return misleadingly empty output rather than a bounded one.

Because journald records the process's immediate slice, the field match is
exact rather than transitive: a grant on `system.slice` does not itself return
entries from units parked in a nested `system-<child>.slice`, and reaching
those requires granting the child slice name, which then exposes that child's
entire membership in the same way. Slice membership is a runtime property of
the host's unit files, drop-ins, and generators — it is not derivable from the
grant list — so operators MUST treat a `read` grant on any `.slice` unit as
equivalent to granting journal read on that slice's whole current and future
membership, and MUST withhold the grant whenever the set of units that could be
placed in the slice is not itself trusted for journal disclosure. This affects
journal reads only; it does not widen `systemctl` unit selection or
`list-units` enumeration, which stay bound to the exact granted names.

The only deletion exception is `JournalCleaner.VacuumJournal`. It is available
only through trusted systemd target configuration and a validated
`JournalVacuumRequest`. Vacuum thresholds come from the command request rather
than a separate operator policy. Every deletion request includes an absolute
modification-time cutoff, and size-based cleanup additionally includes an
allocated-byte target and is rejected without the cutoff. The allocated-byte
target applies to the total allocated bytes of every journal file in the
configured directories, active and archived alike — the same accounting the
disk-usage report uses — while the deletable set remains strictly the archived
files at or before the cutoff. The target therefore bounds total journal
storage and may be unreachable; results MUST report the remaining allocated
bytes rather than imply the target was met. The backend pins each
configured journal directory with `os.Root`, checks directory identity across
open, accepts only strict systemd archived-file names, excludes symlinks and
hardlinks, and revalidates file identity immediately before rooted removal.
Active files, malformed files, and files newer than the request cutoff must
never be deleted. Fixed discovery bounds, cancellation checks, and
partial-progress errors bound each cleanup invocation, in addition to the exact
`systemd-journald.service:clean` authorization and remediation-mode requirement.

The `AllowedSystemServices` action wildcard `*` MUST be accepted only as runner
configuration and MUST expand to the complete canonical set of actions supported
by the running version, including actions added in future versions. Runtime
authorization requests MUST continue to require one concrete supported action,
and effective-policy reporting MUST list the expanded concrete actions. A
wildcard MUST NOT broaden the exact unit selector, bypass remediation mode, or
enable commands outside the fixed builtin surfaces.

`JournalRotator.RotateJournal` is the only journal-daemon mutation exception.
It may call only the fixed `io.systemd.Journal.Rotate` Varlink method through
the configured journal control socket. The backend validates the target machine
ID before connecting, pins the socket inode without following a final symlink,
and connects through the pinned descriptor so a concurrent path replacement
cannot redirect the request. It applies fixed response-size and execution time
bounds. A generic Varlink method or parameter interface must not be exposed to
builtins.

Restricted system-manager access MUST use the public system D-Bus endpoint
selected by `SystemdTargetConfig.ManagerBusSocket` (locally,
`/run/dbus/system_bus_socket`). The private `/run/systemd/private` endpoint is
not a supported API. On Linux, the backend MUST reject a symlinked final socket,
pin its inode before connecting, connect through the pinned descriptor, perform
D-Bus authentication and `Hello`, call
`org.freedesktop.DBus.Peer.GetMachineId` on `org.freedesktop.systemd1` at the
fixed `/org/freedesktop/systemd1` manager object, and compare the result with the
configured machine ID before issuing any manager-interface request. Missing
manager configuration, machine-ID mismatch, path replacement, malformed
authentication, and unsupported platforms MUST fail closed.

Builtins MUST receive only the structured `SystemServiceStateReader` and
`SystemServiceController` capabilities. A raw D-Bus connection, bus name,
object path, interface/member selector, property name, signal subscription, or
method-parameter interface MUST NOT be exposed. The backend may use only the
fixed manager and properties methods required for bounded list/status
inspection, runtime `start`/`stop`/`reload`/`restart` jobs, and persistent
`enable`/`disable` operations.
It MUST apply fixed message, field, result-count, outstanding-call, job-wait,
execution-time, and cancellation bounds. Runtime jobs MUST use the fixed
`replace` job mode, match completion to the returned job identity, wait
synchronously, and report failed, canceled, timed-out, or disconnected jobs as
errors.

Every systemctl operand MUST be an exact, fully suffixed systemd unit name of at
most 255 bytes; implicit `.service` suffixes, glob patterns, aliases resolved by
rshell, and unrestricted manager enumeration are forbidden. An exact configured
selector may itself be a systemd alias and may be resolved by the public manager
API, but output MUST retain the requested/granted selector and the canonical ID
MUST NOT be inserted into or treated as an additional policy grant. All valid
unit types may be selected, including `.service`, `.timer`, and `.socket`. A list
request MUST be constructed solely from the sorted exact names carrying a `read`
grant, with no more than 32 names, so units outside the capability map are
never enumerated. Without `--all`, list processing may consider only already
loaded candidates and return those that are active, failed, or carrying a job.
With `--all`, the backend may load valid read-granted candidates and include
inactive units, while omitting genuinely nonexistent names. Inspection may
return only the fixed, bounded state fields declared by `SystemServiceState`;
status output MUST NOT contain journal records, process command lines, arbitrary
properties, unit-file paths, or D-Bus object paths.

Before any grant lookup, authorization, or manager access, the systemctl
builtin MUST require remediation mode. This command-wide gate applies to bare
and explicit `list-units`, `status`, every mutation, and direct
`systemctl --help`; read-only mode MUST produce no manager or policy
capability call. This does not change the shared non-mutating `read` action,
which remains available to bounded journalctl queries outside remediation mode.
After the mode gate, the builtin MUST expose exactly `list-units`, `status`,
`start`, `stop`, `reload`, `restart`, `enable`, and `disable`. `show`, every
`is-*` predicate, conditional restart variants, `reset-failed`, and `--now`
MUST remain unsupported. The builtin MUST validate the complete request and
authorize every exact unit/action pair. Runtime `start`, `stop`, `reload`, and
`restart` jobs and persistent `enable`/`disable` are structured remediation
capabilities and require their exact grants. Authorization for every requested
unit MUST finish before the first effect. A later systemd failure may
leave earlier authorized effects complete, so backends and builtins MUST return
partial-progress errors rather than imply rollback. An exact runtime grant
authorizes the directly named anchor unit, but the trusted backend may permit
systemd to act on dependency-related units through its normal transaction
semantics. The fixed enable/disable methods may permit systemd to follow
`[Install]` `Alias=`, `Also=`, and template `DefaultInstance=` metadata,
creating or removing installation state for auxiliary or instantiated units.
They also perform a fixed global `Manager.Reload` after the unit-file operation,
which may re-read unrelated host unit changes and run generators. These
indirect, manager-controlled effects MUST be documented and MUST NOT be
generalized into arbitrary dependency, path, alias, or reload parameters.
Standalone `daemon-reload` remains unsupported. The `clean` action remains
restricted to the journal exceptions above and MUST NOT authorize systemctl's
general cleanup operation.

The embedding operator MUST treat each granted unit's configured payload,
aliases, and dependency graph as trusted. Omitting dedicated lifecycle verbs
does not prevent an explicitly granted target, service, alias, or dependency
from rebooting, shutting down, suspending, or otherwise changing host state;
operators MUST withhold those anchor grants when such effects are forbidden.

---

## Implementation Rules

### File System Safety
- Commands MUST NOT write to any files on the system except through an explicitly documented, structured remediation capability such as `TruncateToZeroIfAtLeast`, `JournalCleaner`, or the fixed enable/disable methods on `SystemServiceController`
- Commands MUST NOT execute any files or external binaries on the system in any way
- Commands MUST NOT create, modify, or delete files, directories, or symlinks except as explicitly permitted by such a remediation capability
- Commands MUST NOT follow symlinks during write operations (no writes = no risk, but verify)

### Memory Safety & Resource Limits
- Commands MUST use bounded buffers when reading input (never allocate based on untrusted input size)
- Commands MUST apply backpressure when reading from infinite streams (e.g., stdin from /dev/zero)
- Commands MUST limit memory consumption to prevent exhaustion attacks
- Commands MUST NOT load entire files into memory when line-by-line or chunked processing is viable
- Commands MUST handle very long lines (>1MB) without crashing or excessive memory use
- Commands MUST respect the global 10 MiB output limit (enforced by executor, but don't generate excess)

### Input Validation & Error Handling
- Commands MUST validate all numeric arguments (line counts, byte counts, field numbers) for overflow
- Commands MUST reject negative values where semantically invalid
- Commands MUST reject all disallowed flags with clear error messages
- Commands MUST fail safely on malformed or binary input (no crashes, no hangs)
- Commands MUST return proper exit codes (0 = success, 1 = error, as appropriate)
- Commands MUST write error messages to stderr, not stdout

### Special File Handling
- Commands MUST handle /dev/zero, /dev/random, and similar infinite sources safely (bounded reads, timeout respected)
- Commands MUST NOT block indefinitely when reading from FIFOs or pipes
- Commands MUST handle /proc and /sys files appropriately (short reads, non-seekable)
- Commands MUST handle non-regular files (directories, devices, sockets) with appropriate errors

### Regular Expression Safety (for grep, sed, etc.)
- Regex engines MUST use bounded execution to prevent ReDoS attacks
- Commands MUST timeout or limit backtracking on pathological regex patterns
- Commands MUST validate regex syntax before execution
- Commands SHOULD prefer linear-time regex engines (e.g., RE2) over backtracking engines where possible

### Path & Traversal Safety
- Commands MUST resolve relative paths correctly relative to working directory
- Commands MUST handle path traversal (../) without restriction (by design - rely on OS permissions)
- Commands MUST NOT be confused by absolute paths, multiple slashes, or . and .. components
- Commands MUST handle paths with special characters (spaces, newlines, unicode) correctly
- Commands MUST follow symlinks for read operations (by design - document this behavior)

### Concurrency & Race Conditions
- Commands MUST use atomic operations where ordering matters
- Commands MUST NOT be vulnerable to TOCTOU races (already mitigated by no-write policy)
- Commands MUST be safe for concurrent execution (no shared mutable state)

### Denial of Service Prevention
- Commands MUST respect configured execution timeouts
- Commands MUST NOT enter infinite loops on any input
- Commands MUST NOT cause excessive CPU usage through algorithmic complexity attacks
- Commands MUST NOT exhaust file descriptors or other system resources
- Commands MUST handle interruption (context cancellation) gracefully

### Goroutine Context Propagation
- Goroutines spawned during command or interpreter execution MUST propagate and respect context cancellation. Before any blocking I/O in a goroutine, check `ctx.Err()`. For multi-chunk writes, check `ctx.Err()` between chunks. Close the pipe write end (`pw.Close()`) on cancellation to unblock the reader and propagate termination.

### Integer Safety
- Commands MUST check for integer overflow in all arithmetic operations
- Commands MUST validate numeric conversions (string to int) and handle errors
- Commands MUST handle edge cases (INT_MAX, 0, negative numbers) correctly

### Output Consistency
- Commands MUST produce deterministic output for the same input
- Commands MUST NOT leak sensitive information through error messages or timing
- Commands MUST handle line endings consistently (\n, \r\n, \r)
- Commands MUST preserve or correctly handle binary data (non-UTF8)

### Process Introspection Privacy
- Commands that inspect host processes MUST NOT read or expose raw process argv / full command lines (for example `/proc/<pid>/cmdline`, macOS `KERN_PROCARGS*`, Windows command-line APIs, or equivalent sources)
- Process-listing commands such as `ps` MUST report only process name / executable metadata unless an explicit security-reviewed exception is documented
- Any exception that exposes argv-like data MUST define a redaction strategy and include tests proving sensitive arguments are not emitted

### Environment Privacy
- Builtins and interpreter runtime code MUST NOT read the host process environment directly via `os.Getenv`, `os.LookupEnv`, `os.Environ`, or platform-specific environment APIs.
- Commands that inspect host processes MUST NOT read or expose raw process environment blocks, such as `/proc/<pid>/environ`, macOS process environment APIs, Windows PEB environment blocks, or equivalent sources.

### Testing Requirements
- Every dangerous flag MUST have a test verifying it is rejected
- Every supported flag MUST have correctness tests
- Edge cases (empty files, no trailing newline, single line) MUST be tested
- Error paths (missing file, invalid args) MUST be tested
- Security properties MUST be tested (path traversal, special files, large inputs)
- Integration with shell features (pipes, for-loops, globs) MUST be tested

### Cross-Platform Compatibility (Linux, macOS, Windows)

#### General Path & File Handling
- Commands MUST use `filepath` package functions for all path operations (never hardcode `/` or `\`)
- Commands MUST use `filepath.Join()` to construct paths, NOT string concatenation
- Commands MUST use `filepath.Clean()` to normalize paths before validation
- Commands MUST handle case-insensitive filesystems (Windows/macOS default) vs case-sensitive (Linux)
- Commands MUST handle Windows drive letters (e.g., `C:\path`) and UNC paths (e.g., `\\server\share`)
- Commands MUST NOT exceed platform path length limits (historically 260 chars on Windows, verify with long path tests)

#### Windows-Specific Issues
- Commands MUST detect and reject Windows reserved filenames (CON, PRN, AUX, NUL, COM1-9, LPT1-9) to prevent hangs and security issues
- Commands MUST be aware that Windows file locking is mandatory (not advisory like Unix) - reads may fail on locked files
- Commands SHOULD validate for Windows Alternate Data Streams (ADS) syntax (`filename:stream`) and document behavior
- Commands MUST handle Windows line endings (CRLF `\r\n`) in addition to Unix (LF `\n`)
- Commands SHOULD document behavior when encountering ADS (reject, ignore stream suffix, or read the stream)

#### macOS-Specific Issues
- Commands MUST handle macOS Unicode NFD normalization (decomposed form) vs NFC on Linux/Windows
- Commands SHOULD normalize Unicode when comparing filenames (use `golang.org/x/text/unicode/norm` if needed)
- Commands MAY ignore AppleDouble files (`._*`) created by macOS on non-HFS filesystems (document if ignored)
- Commands SHOULD be aware that macOS extended attributes (xattrs) may exceed 64KB causing issues on Linux
- Commands MUST NOT assume resource forks or xattrs are preserved when reading files

#### Unix-Specific Issues
- Commands MUST NOT assume Unix-specific paths (e.g., `/dev/zero`, `/proc`) exist on all platforms
- Commands testing special files SHOULD use build tags to skip tests on platforms without those files
- Commands relying on Unix permissions MUST handle Windows ACL differences gracefully

#### Line Ending Handling
- Commands MUST handle all three line ending formats: LF (`\n`), CRLF (`\r\n`), and CR (`\r`)
- Commands SHOULD normalize line endings when processing line-oriented data (or document which format is expected)
- Commands that count lines MUST handle mixed line endings (e.g., some `\n`, some `\r\n` in same file)

#### Testing Requirements
- Tests MUST be runnable on Linux, macOS, and Windows without modification
- Tests MUST use `filepath.Join()` for expected paths, never hardcoded separators
- Tests MUST NOT rely on Unix-specific files (/dev/null) or behaviors without platform checks
- Tests SHOULD use build tags (`//go:build unix` or `//go:build windows`) for platform-specific test cases
- Tests SHOULD verify Unicode NFD/NFC handling if processing filenames with accented characters
- Tests SHOULD include Windows reserved filename rejection tests
- Tests SHOULD verify behavior with locked files on Windows (use build tags)

---

## Sources & References

Research based on:

**General Security:**
- [GNU Coreutils CVE Database](https://www.cvedetails.com/product/5075/GNU-Coreutils.html)
- [OWASP ReDoS Attack](https://owasp.org/www-community/attacks/Regular_expression_Denial_of_Service_-_ReDoS)
- [Symlink TOCTOU Attacks](https://medium.com/@instatunnel/symlink-attacks-when-file-operations-betray-your-trust-986d5c761388)
- [Wikipedia: Time-of-check to time-of-use](https://en.wikipedia.org/wiki/Time-of-check_to_time-of-use)
- [Special Device Files](https://en.wikipedia.org/wiki//dev/zero)

**Cross-Platform Path Handling:**
- [Go filepath Package Documentation](https://pkg.go.dev/path/filepath)
- [Cross-Platform Go File Paths](https://www.slingacademy.com/article/exploring-the-path-filepath-package-for-cross-platform-file-path-handling-in-go/)
- [Line Ending Types: LF vs CRLF vs CR](https://www.baeldung.com/linux/line-breaks-types)

**Windows-Specific:**
- [Windows Reserved Filenames](https://www.meziantou.net/reserved-filenames-on-windows-con-prn-aux-nul.htm)
- [CVE-2025-27210: Windows Device Name Path Traversal](https://zeropath.com/blog/cve-2025-27210-nodejs-path-traversal-windows)
- [Windows Alternate Data Streams Security](https://blog.netwrix.com/2022/12/16/alternate_data_stream/)
- [Malware Hiding Using ADS](https://denwp.com/unveiling-the-stealth/)
- [File Locking: Mandatory vs Advisory](https://en.wikipedia.org/wiki/File_locking)
- [Microsoft: Enabling Advisory File Locking](https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-server-2003/cc778584(v=ws.10))

**macOS-Specific:**
- [macOS Unicode Normalization (NFD vs NFC)](https://eclecticlight.co/2021/05/08/explainer-unicode-normalization-and-apfs/)
- [UTF-8 Hell of Mac OSX](https://medium.com/@sthadewald/the-utf-8-hell-of-mac-osx-feef5ea42407)
- [APFS Extended Attributes](https://eclecticlight.co/2024/05/13/apfs-extended-attributes-revisited/)
- [Resource Forks Wikipedia](https://en.wikipedia.org/wiki/Resource_fork)
- [macOS xattr Deep Dive](https://rndt.pages.dev/posts/2023-calendar/d01/macos-xattr/)
