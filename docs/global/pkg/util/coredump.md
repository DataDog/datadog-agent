# pkg/util/coredump

## Purpose

`pkg/util/coredump` provides a single cross-platform entry point, `Setup(cfg)`, that enables OS-level core dumps for agent processes when the `go_core_dump` configuration key is set to `true`. On Linux/macOS it removes the `RLIMIT_CORE` resource limit and sets Go's crash traceback to `"crash"` mode so that a fatal signal produces a full core file. On Windows the feature is not supported and `Setup` returns an error if the key is enabled.

---

## Key elements

**`Setup(cfg model.Reader) error`** — reads `go_core_dump` from the agent config. If `true`:

- **Linux / macOS** (`!windows` build tag):
  1. Calls `debug.SetTraceback("crash")` — instructs the Go runtime to dump a full traceback on crash (instead of the default abbreviated form) and to not recover panics, which is required for the OS to generate a core file.
  2. Calls `unix.Setrlimit(RLIMIT_CORE, {Cur: RLIM_INFINITY, Max: RLIM_INFINITY})` — removes the soft and hard limits on core file size.
  Returns an error if `setrlimit` fails (e.g. insufficient privileges).

- **Windows** (`windows` build tag): returns `errors.New("Not supported on Windows")`.

If `go_core_dump` is `false` (the default), `Setup` is a no-op on all platforms.

---

## Usage

`Setup` is called once at process startup in every major agent binary, immediately after configuration is loaded:

- `cmd/agent/subcommands/run/command.go`
- `cmd/system-probe/subcommands/run/command.go`
- `cmd/security-agent/subcommands/start/command.go`
- `cmd/process-agent/command/main_common.go`
- `cmd/dogstatsd/subcommands/start/command.go`
- `cmd/cluster-agent/subcommands/start/command.go`
- `comp/trace/agent/impl/run.go`

Typical call site:

```go
if err := coredump.Setup(config.Datadog()); err != nil {
    log.Warnf("Can't enable core dumps: %v", err)
}
```

Errors from `Setup` are treated as warnings rather than fatal, so the agent continues to run even if core dumps cannot be enabled (e.g. when running as a non-root user).

### Enabling core dumps

Add to `datadog.yaml`:

```yaml
go_core_dump: true
```

On Linux, also ensure the kernel's `core_pattern` (e.g. `/proc/sys/kernel/core_pattern`) points to a writable location and that the process has sufficient disk quota.

---

## Relationship to other packages

| Package | Relationship |
|---|---|
| `pkg/util/filesystem` ([docs](filesystem.md)) | Core dump files are written to directories managed by the OS kernel. `pkg/util/filesystem` provides helpers (`FileExists`, `GetFileSize`, disk-usage utilities) that can be used to check available space or detect whether a core file was produced. The two packages do not directly depend on each other. |
| `pkg/runtime` ([docs](../runtime.md)) | Both packages tune low-level runtime behavior at startup. `pkg/runtime` calls `debug.SetTraceback` via `go.uber.org/automaxprocs` (for GOMAXPROCS/GOMEMLIMIT), while `pkg/util/coredump` calls `debug.SetTraceback("crash")` specifically to ensure core files are generated on fatal crashes. The call ordering matters: `coredump.Setup` should be called before any goroutine could panic, but after the config is loaded. |
