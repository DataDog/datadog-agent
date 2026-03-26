# pkg/security/ptracer — ptrace-based CWS tracer (eBPF-less mode)

## Purpose

`pkg/security/ptracer` implements the **CWS injector**: a ptrace-based syscall interceptor used as an alternative to eBPF when eBPF is unavailable in the target environment (e.g., constrained containers or older kernels). It wraps or attaches to a tracee process, intercepts syscalls using Linux `ptrace(2)` and optionally a seccomp filter, converts each intercepted event into an `ebpfless.SyscallMsg` protobuf message, and forwards those messages over a TCP connection to the `system-probe`'s `EBPFLessProbe`. From the rest of CWS's perspective, events produced by the ptracer are indistinguishable from eBPF-sourced events.

The package is **Linux-only** (`//go:build linux`). It is exposed as a binary through `cmd/cws-instrumentation`, invoked as a sidecar or entrypoint wrapper inside a container.

## Key elements

### Core types

| Type | File | Description |
|------|------|-------------|
| `Tracer` | `ptracer.go` | Base struct holding the PID list, syscall handler map, architecture info (`arch.Info`), and user/group caches. Implements low-level memory reads (`ReadArgString`, `ReadArgUint64`, etc.) using `process_vm_readv(2)`. |
| `CWSPtracerCtx` | `cws.go` | Top-level context embedding `Tracer`. Manages the connection to `system-probe`, the message queue (`msgDataChan`), the process cache (`ProcessCache`), credentials, container ID, and cancellation. |
| `Opts` | `cws.go` | Configuration passed at startup: `Creds`, `Verbose`, `Debug`, `Async`, `StatsDisabled`, `ProcScanDisabled`, `ScanProcEvery`, `SeccompDisabled`, `AttachedCb`. |
| `Creds` | `ptracer.go` | Optional UID/GID override (`*uint32`) applied to the tracee at fork time. |
| `ProcProcess` | `proc.go` | Wraps `gopsutil/v4/process.Process` with a `CreateTime` field; used during procfs scans. |

### Callback types

| Constant | Meaning |
|----------|---------|
| `CallbackPreType` | Fired at syscall entry (before the kernel executes it). |
| `CallbackPostType` | Fired at syscall exit (after the return value is available). |
| `CallbackExitType` | Fired when a traced PID exits. |

### Syscall handler registration

`registerSyscallHandlers()` in `cws.go` aggregates three handler groups:

| Function | File | Coverage |
|----------|------|---------|
| `registerFIMHandlers` | `fim_handlers.go` | File operations: `open`, `openat`, `openat2`, `creat`, `name_to_handle_at`, `open_by_handle_at`, `unlink`, `unlinkat`, `rename`, `renameat`, `renameat2`, `mkdir`, `mkdirat`, `rmdir`, `chmod`, `fchmod`, `fchmodat`, `chown`, `fchown`, `lchown`, `fchownat`, `link`, `linkat`, `symlink`, `symlinkat`, `utimes`, `utimensat`. |
| `registerProcessHandlers` | `process_handlers.go` | Process lifecycle: `execve`, `execveat`, `chdir`, `fchdir`, `setuid`, `setgid`, `setresuid`, `setresgid`, `capset`, `prctl`. |
| `registerNetworkHandlers` | `network_handlers.go` | Network: `bind`. |
| `registerERPCHandlers` | `erpc.go` | eRPC tunnel over `ioctl`. |

Each handler is a `syscallHandler` struct with optional `Func` (pre-hook), `ShouldSend` (filter on return value), and `RetFunc` (post-hook) fields.

### Key functions

| Function | Description |
|----------|-------------|
| `Wrap(args, envs, probeAddr, opts)` | Forks the tracee under ptrace, attaches seccomp filter if enabled, then calls `StartCWSPtracer`. Entry point for the **wrapped** mode. |
| `Attach(pids, probeAddr, opts)` | Attaches to already-running PIDs. Entry point for the **attach** mode. Seccomp is always disabled in this mode. |
| `(ctx) NewTracer()` | Compiles the seccomp BPF filter from `PtracedSyscalls`, forks the tracee via `forkExec`, and sets `PTRACE_O_*` options. |
| `(ctx) AttachTracer()` | Calls `PTRACE_ATTACH` on each PID and sets `PTRACE_O_*` options. |
| `(ctx) Trace()` | Main tracing loop. Dispatches to `trace()` (full-ptrace) or `traceWithSeccomp()` depending on `opts.SeccompDisabled`. |
| `(ctx) StartCWSPtracer()` | Initializes the upstream TCP connection, starts the procfs scanner, and calls `Trace()`. |
| `(ctx) CWSCleanup()` | Cancels context, drains the message queue, and closes the connection. |
| `forkExec1` | Low-level `clone(2)` + `execve(2)` in a `//go:nosplit` function; calls `PTRACE_TRACEME` and applies seccomp in the child before exec. |

### Environment variable overrides (testing)

| Variable | Effect |
|----------|--------|
| `TEST_DD_PASSWD_PATH` (`EnvPasswdPathOverride`) | Override `/etc/passwd` path for user resolution. |
| `TEST_DD_GROUP_PATH` (`EnvGroupPathOverride`) | Override `/etc/group` path for group resolution. |

### Build flags

The entire package is gated on `//go:build linux`. There are no additional build tags.

## Usage

The package is consumed exclusively by the `cws-instrumentation` binary:

```
cmd/cws-instrumentation/subcommands/tracecmd/trace.go
```

**Wrap mode** (inject as entrypoint):

```go
opts := ptracer.Opts{
    Creds:           ptracer.Creds{UID: &uid, GID: &gid},
    Async:           true,
    SeccompDisabled: false,
}
exitCode, err := ptracer.Wrap(os.Args[1:], os.Environ(), "127.0.0.1:5678", opts)
```

**Attach mode** (attach to existing PIDs):

```go
err := ptracer.Attach([]int{pid1, pid2}, "127.0.0.1:5678", opts)
```

In both modes `probeAddr` points to the `EBPFLessProbe` TCP listener inside `system-probe`. If `probeAddr` is empty, events are silently discarded (useful in tests). The ptracer sends an initial `MessageTypeHello` handshake, then streams `MessageTypeSyscall` messages encoded with msgpack.

The package is also exercise-tested via `pkg/security/tests/ebpfless_test.go` and `pkg/security/tests/main_linux.go`.

### Integration with EBPFLessProbe

The complete eBPF-less data path is:

```
cws-instrumentation (ptracer)
  └─► ptrace(2) intercepts syscall
        └─► syscallHandler: convert to ebpfless.SyscallMsg (msgpack)
              └─► TCP connection to EBPFLessProbe listener (system-probe)
                    └─► EBPFLessProbe.handleEvent
                          └─► Probe.DispatchEvent  (same path as eBPF events)
                                └─► RuleEngine.HandleEvent
```

From `RuleEngine`'s perspective, events from `EBPFLessProbe` are identical to eBPF events — both arrive as `model.Event` values. The origin is recorded as `event.Origin = "ebpfless"` (constant `EBPFLessOrigin` from `pkg/security/probe`), which can be matched in SECL rule `filters:` via the `origin` field (see [secl-model.md](secl-model.md) and [probe.md](probe.md)).

### ProcessCache and procfs scanning

`CWSPtracerCtx` maintains its own `ProcessCache` for user-space process ancestry tracking (separate from the kernel-side `process/EBPFResolver` in `EBPFResolvers`). When `ProcScanDisabled` is false, the tracer periodically scans `/proc` (every `ScanProcEvery` interval) to snapshot already-running processes that were not caught via ptrace. This mirrors the resolver snapshot performed by `EBPFResolvers.Snapshot()` on the probe side (see [resolvers.md](resolvers.md)).

### UID/GID resolution

User and group names are resolved from `/etc/passwd` and `/etc/group` via the `usergrouputils` parsers in `pkg/security/common`. The env-var overrides `TEST_DD_PASSWD_PATH` and `TEST_DD_GROUP_PATH` (using the same constants exported from this package) allow the test suite to inject a synthetic passwd/group file without root access.

### eRPC tunnel

`registerERPCHandlers` intercepts `ioctl` calls on a specific file descriptor to implement an eRPC (embedded RPC) tunnel. This tunnel is also used by the eBPF probe (see `pkg/security/probe/erpc/`) and allows the kernel side to request user-space actions (e.g., discarder invalidation) even in eBPF-less mode.

### SECL model coverage

Each intercepted syscall is converted to one of the following SECL event types (defined in `pkg/security/secl/model`):

| Handler group | SECL event types |
|---|---|
| FIM | `open`, `unlink`, `rename`, `mkdir`, `rmdir`, `chmod`, `chown`, `link`, `symlink`, `utimes` |
| Process | `exec`, `chdir`, `setuid`, `setgid`, `capset`, `prctl` |
| Network | `bind` |

See [secl-model.md](secl-model.md) for the full `EventType` list.

## Related documentation

| Doc | Description |
|-----|-------------|
| [probe.md](probe.md) | `EBPFLessProbe` is the system-probe counterpart: it opens the TCP listener, decodes `SyscallMsg` messages, and dispatches them through `Probe.DispatchEvent`. The eBPF-less mode section in that doc describes the full integration. |
| [secl-model.md](secl-model.md) | `model.Event` struct and `EventType` constants that `SyscallMsg` values are translated into; `origin` field used to distinguish `"ebpfless"` events from eBPF events. |
| [security.md](security.md) | Top-level CWS architecture: ptracer is listed as the eBPF-less probe path; the overall event flow is shown end-to-end. |
| [resolvers.md](resolvers.md) | `EBPFResolvers` on the system-probe side provides the same resolver infrastructure for eBPF-less events as for eBPF events. |
| [common.md](common.md) | `usergrouputils` sub-package provides the passwd/group parsers used for UID/GID resolution inside the ptracer. |
