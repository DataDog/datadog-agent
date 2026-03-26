# pkg/collector/python

## Purpose

`pkg/collector/python` embeds a CPython 3 interpreter inside the Agent process and provides the machinery to load, configure, and run Python-based integration checks. It is the bridge between Go (the agent core) and the `datadog_checks` Python ecosystem.

All Python checks—anything in `datadog_checks.*` wheels or in custom `conf.d/` check directories—pass through this package. Without it, only native Go checks are available.

## Key elements

### Build tags

| Tag | Effect |
|-----|--------|
| `python` | Enables the entire package. All files that contain Cgo code are guarded with `//go:build python`. Files with `//go:build !python` provide no-op stubs. |

Every file that calls into the Python C API or `rtloader` carries `//go:build python` and uses `import "C"`.

### rtloader (`/rtloader/`)

The package delegates all direct CPython calls to **rtloader**, a thin C/C++ shared library (`libdatadog-agent-rtloader`) built from `rtloader/`. The Go code interacts with rtloader through CGo. The linker flags are set in `init.go`:

```
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows  LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
```

### Initialization (`init.go`, `embed_python.go`)

- `Initialize(paths ...string) error` — creates the Python 3 interpreter via `C.make3(...)`, sets `PYTHONPATH`, wires up all Go-side callbacks, and calls `C.init(rtloader)`. Should be called once at agent startup.
- `InitPython(paths ...string)` — thin wrapper around `Initialize`; also calls `pyPrepareEnv()` to configure `psutil.PROCFS_PATH` if `procfs_path` is set. This is the entry point used by agent startup code.
- `InitPython` is a no-op when compiled without the `python` build tag (`embed_nopy.go`).
- Exported package variables `PythonVersion`, `PythonHome`, `PythonPath` are set after successful initialization and are exposed through the Agent status and metadata subsystems.

### Callback modules registered into rtloader

During `Initialize`, Go functions are registered as C callbacks so that Python check code can call back into the Agent runtime:

| Module | Go source | Provides to Python |
|--------|-----------|--------------------|
| `datadog_agent` | `datadog_agent.go` | `get_hostname`, `get_config`, `get_version`, `send_log`, `obfuscate_sql`, `emit_agent_telemetry`, etc. |
| `aggregator` | `pkg/collector/aggregator` | `submit_metric`, `submit_event`, `submit_service_check`, etc. |
| `_util` | `subprocesses.go` | `get_subprocess_output` |
| `tagger` | `tagger.go` | `get_tags` |
| `containers` | `containers.go` | `is_excluded` |
| `kubeutil` | `kubeutil.go` | `get_connection_info` |

### PythonCheckLoader (`loader.go`)

`PythonCheckLoader` implements `check.Loader` (priority 20). It is registered automatically via `init()` and is selected by the collector when no Go loader claims a check name.

Key method: `Load(senderManager, config, instance, instanceIndex) (check.Check, error)`

1. Imports the module as `datadog_checks.<name>` (wheel path), falling back to bare `<name>`.
2. Reads `__version__` for the wheel version.
3. Optionally runs the `a7` Python 3 compatibility linter in a goroutine (`reportPy3Warnings`).
4. Reads `HA_SUPPORTED` class attribute when HA agent mode is enabled.
5. Creates a `PythonCheck` and calls `Configure`.

Expvar keys under `pyLoader`: `ConfigureErrors`, `Py3Warnings`.

### PythonCheck (`check.go`)

`PythonCheck` implements `check.Check`. It holds two CGo pointers: `class` (the Python class object) and `instance` (the instantiated check object). The check lifecycle is:

| Method | Behaviour |
|--------|-----------|
| `Configure(...)` | Acquires GIL, calls `C.get_check` (new API) or `C.get_check_deprecated` (legacy), sets interval, scrubs config. |
| `Run()` | Acquires GIL, calls `C.run_check`, commits metrics via the sender, collects warnings. |
| `RunSimple()` | Same as `Run` but skips `Commit`; used for ad-hoc diagnosis. |
| `Cancel()` | Calls `C.cancel_check`, marks check as cancelled. |
| `GetDiagnoses()` | Calls `C.get_check_diagnoses`, returns JSON-deserialized `[]diagnose.Diagnosis`. |

A Go finalizer (`pythonCheckFinalizer`) decrements PyObject reference counts asynchronously when the check is garbage collected.

### GIL management (`helpers.go`)

`stickyLock` wraps `rtloader_gilstate_t` and pairs every GIL acquire with `runtime.LockOSThread()` / `runtime.UnlockOSThread()`. This prevents the Go scheduler from migrating a goroutine between threads while the GIL is held, which would cause a Python runtime assertion failure.

- `newStickyLock() (*stickyLock, error)` — acquires GIL; returns `ErrNotInitialized` if `rtloader` is nil.
- `(*stickyLock).unlock()` — releases GIL and unlocks the OS thread.

### Memory tracking (`memory.go`)

- `TrackedCString(str string) *C.char` — default implementation is `C.CString`. When `memtrack_enabled` is set, it is replaced with a variant that reports allocation sizes to Prometheus counters under the `rtloader` subsystem.
- `InitMemoryTracker()` — activates the rtloader memory tracker (`C.enable_memory_tracker`) and starts a goroutine that polls `C.get_and_reset_memory_stats` every second.
- Python heap memory is tracked separately via `initPymemTelemetry` (gauges `pymem_inuse`, counter `pymem_alloc`).

### Utility functions (`helpers.go`)

- `GetPythonIntegrationList() ([]string, error)` — returns installed `datadog_checks.*` package names (excluding dev/test wheels).
- `GetPythonInterpreterMemoryUsage() ([]*PythonStats, error)` — snapshot of CPython object counts and sizes.
- `SetPythonPsutilProcPath(path string) error` — sets `psutil.PROCFS_PATH` for containerized environments with a non-standard procfs.

## Usage

### In the agent startup path

`cmd/agent/subcommands/run/command.go` calls `python.InitPython(common.GetPythonPaths()...)` before starting the collector. When `python_lazy_loading` is enabled in `datadog.yaml`, `InitPython` is deferred to the first `Load` call inside `PythonCheckLoader`.

### In the collector

`comp/collector/collector/collectorimpl/collector.go` imports the package (with the `python` build tag) so that the loader's `init()` registration runs. The `PythonCheckLoader` then participates in the loader priority chain alongside `GoCheckLoader` and `JMXCheckLoader`.

### In metadata and status

`comp/metadata/host/hostimpl/utils/host_nix.go` and the Windows equivalent call `python.GetPythonIntegrationList()` and read `python.PythonVersion` / `python.PythonHome` for the `python` section of the host metadata payload. The agent status page reads the same expvars.

### Relevant configuration keys

| Key | Description |
|-----|-------------|
| `python_lazy_loading` | Defer interpreter init to first check load |
| `memtrack_enabled` | Enable Cgo memory tracking |
| `telemetry.python_memory` | Enable `pymem_*` telemetry |
| `disable_py3_validation` | Skip Python 3 compatibility linting |
| `allow_python_path_heuristics_failure` | Tolerate interpreter path resolution failures |
| `procfs_path` | Override `psutil.PROCFS_PATH` inside the interpreter |

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/check`](check.md) | `PythonCheck` implements the `Check` interface defined here. `PythonCheckLoader` implements the `Loader` interface (priority 20). `ErrSkipCheckInstance` is returned by `PythonCheckLoader` when it cannot handle a config that another loader should pick up instead. |
| [`pkg/collector/loaders`](loaders.md) | `PythonCheckLoader` registers itself via `init()` in `loader.go`. The catalog is built once after all `init()` functions have run. Priority 20 means Python checks are tried before Go core checks (30) but after any custom high-priority loaders. |
| [`pkg/collector/corechecks`](corechecks.md) | Go core checks (`GoCheckLoader`, priority 30) run in the same loader chain. When both a Python wheel and a Go implementation exist for the same check name, the Go check can call `check.ErrSkipCheckInstance` to defer to the Python version based on the config content. |
| [`pkg/aggregator/sender`](../aggregator/sender.md) | The `aggregator` callback module registered during `Initialize` bridges Python's `self.gauge(...)` / `self.event(...)` etc. to the Go `Sender` interface. Each `PythonCheck.Run()` ends with `sender.Commit()` just like a Go check. |
| [`pkg/tagger`](../tagger.md) | The `tagger` callback module exposes `get_tags(entity_id, cardinality)` to Python checks. This allows Python checks to request infrastructure tags for a container ID or other entity without importing the full tagger component. |
