# Python checks and rtloader

-----

Most Datadog integrations are Python classes deriving from `AgentCheck`, shipped as wheels built from the separate [integrations-core](https://github.com/DataDog/integrations-core) repository. The agent does not shell out to a Python process to run them: it embeds a CPython interpreter in the agent process through **rtloader**, a C/C++ shim with a C89 API that Go drives via cgo. This page explains that embedding — initialization and paths, the loader, GIL discipline, the builtin modules backed by Go callbacks, and the wheel supply chain.

Scheduling and execution are the standard [check collector](collector.md) pipeline; a `PythonCheck` is just another implementation of `check.Check`. Everything here is compiled only under the `python` build tag — binaries built without it (slim container images, the Cluster Agent by default) have no Python support at all.

## Key packages

| Path | Purpose |
|---|---|
| [`rtloader`](<<<SRC>>>/rtloader) | Standalone C++ project (CMake, also a Bazel target) producing `libdatadog-agent-rtloader` |
| [`rtloader/include/datadog_agent_rtloader.h`](<<<SRC>>>/rtloader/include/datadog_agent_rtloader.h) | The C API surface consumed by Go (`make3`, `run_check`, `get_class`, `set_*_cb`, ...) |
| [`rtloader/rtloader/api.cpp`](<<<SRC>>>/rtloader/rtloader/api.cpp) | C API implementation; `make3()` dynamically loads the Python 3 backend |
| [`rtloader/three/three.cpp`](<<<SRC>>>/rtloader/three/three.cpp) | The CPython 3 backend (`libdatadog-agent-three`): interpreter lifecycle, GIL, object calls |
| [`rtloader/common/builtins`](<<<SRC>>>/rtloader/common/builtins) | C implementations of the Python builtin modules (`aggregator`, `datadog_agent`, `tagger`, ...) |
| [`pkg/collector/python/init.go`](<<<SRC>>>/pkg/collector/python/init.go) | cgo linkage, interpreter init, callback registration, path resolution |
| [`pkg/collector/python/loader.go`](<<<SRC>>>/pkg/collector/python/loader.go) | `PythonCheckLoader` (loader name `python`, priority 20) |
| [`pkg/collector/python/check.go`](<<<SRC>>>/pkg/collector/python/check.go) | `PythonCheck`: Configure/Run/Cancel against interpreter objects |
| [`pkg/collector/python/helpers.go`](<<<SRC>>>/pkg/collector/python/helpers.go) | `stickyLock`: GIL + `runtime.LockOSThread` management |
| [`pkg/collector/python/datadog_agent.go`](<<<SRC>>>/pkg/collector/python/datadog_agent.go) | Go implementations of the `datadog_agent` builtin functions |
| [`pkg/collector/aggregator`](<<<SRC>>>/pkg/collector/aggregator) | Submit callbacks (`SubmitMetric`, ...) and the check-ID → `Sender` routing (`CheckContext`) |
| [`pkg/collector/python/memory.go`](<<<SRC>>>/pkg/collector/python/memory.go) | rtloader allocation tracking (`memtrack_enabled`) |
| [`pkg/collector/python/subprocesses.go`](<<<SRC>>>/pkg/collector/python/subprocesses.go) | Tracking of subprocesses started by `get_subprocess_output` |
| [`cmd/agent/common/common.go`](<<<SRC>>>/cmd/agent/common/common.go) | `GetPythonPaths()`: PYTHONPATH precedence |
| [`deps/agent_integrations/source_packages.bzl`](<<<SRC>>>/deps/agent_integrations/source_packages.bzl) | Bazel rules building integrations-core wheels from the pinned commit |
| [`cmd/agent/subcommands/integrations/command.go`](<<<SRC>>>/cmd/agent/subcommands/integrations/command.go) | `agent integration install/remove/show/freeze` |

## Architecture: rtloader

rtloader exists to decouple the agent from a specific CPython version and to keep all `Python.h` usage out of Go. The layering:

```text
 Go (pkg/collector/python)          C ABI                     CPython
 ┌───────────────────────┐   ┌──────────────────────┐   ┌──────────────────────┐
 │ PythonCheckLoader     │──▶│ libdatadog-agent-    │──▶│ libdatadog-agent-    │
 │ PythonCheck           │◀──│ rtloader (api.cpp)   │◀──│ three (three.cpp)    │
 │ init.go (cgo)         │   │ C89 API, callbacks   │   │ links libpython3     │
 └───────────────────────┘   └──────────────────────┘   └──────────────────────┘
        ▲                            │ set_submit_metric_cb, set_get_config_cb, ...
        └── exported Go callbacks ◀──┘   (C function pointers into Go)
```

1. Go calls flow *down* through the C API (`run_check`, `get_class`, `get_check`, `get_checks_warnings`, ...).
1. Python builtin modules flow *up*: the C builtins in [`rtloader/common/builtins`](<<<SRC>>>/rtloader/common/builtins) hold function pointers that [`init.go`](<<<SRC>>>/pkg/collector/python/init.go) points at exported Go functions during initialization. When a check calls `datadog_agent.get_config(...)`, it lands in Go.
1. `make3()` loads the `three` backend dynamically, which in turn embeds Python 3. Python 2 support has been removed; the `three` name is a fossil of the 2/3 era.

## Interpreter initialization and paths

[`python.Initialize`](<<<SRC>>>/pkg/collector/python/init.go) runs either at collector construction or, with `python_lazy_loading: true` (the default), on the first Python check load. Steps:

1. Resolve `PythonHome` relative to the agent executable: `<exe>\..\embedded3` on Windows, `<exe>/../../embedded` on Linux and macOS (`/opt/datadog-agent/embedded` in a standard install). If the relative directory does not exist (dev builds, unit tests), fall back to the ldflags-baked path. FIPS-mode agents also point `OPENSSL_CONF`/`OPENSSL_MODULES` into the embedded directory before any native code runs.
1. Call `make3(pythonHome, pythonExecPath)` to create the interpreter.
1. Append the agent's check paths to `sys.path` via `GetPythonPaths()`, in precedence order: the dist path (common modules), the legacy integrations-core checks directory, `<dist>/checks.d`, and `additional_checksd` (default `/etc/datadog-agent/checks.d`) for custom checks. Wheels installed in the embedded interpreter's site-packages are already on `sys.path` and take precedence over all of these.
1. Register every Go callback with rtloader (`initDatadogAgentModule`, `initAggregatorModule`, `initTaggerModule`, ...), then run rtloader's own init, which finalizes the builtin modules.
1. Query `sys.version`/`sys.path` (exposed as `python.PythonVersion`/`PythonPath` and in `agent status`) and emit the recurrent `datadog.agent.python.version` series.

Initialization errors accumulate in the `pythonInit` expvar rather than crashing the agent; a broken Python environment turns every Python check load into a loader error.

## Loading a check

[`PythonCheckLoader.Load`](<<<SRC>>>/pkg/collector/python/loader.go) is invoked by the [check scheduler](collector.md) for each instance the Go loader did not claim:

1. Import the module `datadog_checks.<name>` first, then bare `<name>` as a fallback ([`loader_helpers.go`](<<<SRC>>>/pkg/collector/python/loader_helpers.go)). The first form finds bundled/installed wheels; the second finds custom checks dropped in `checks.d`. This ordering means a custom check only shadows a bundled integration if the wheel is absent — same-named custom checks do not override installed wheels.
1. Find the `AgentCheck` subclass in the module via rtloader `get_class`, read its `__version__` attribute (the wheel version surfaced by `Version()` and `agent status`) and, when `ha_agent.enabled` is set, its `HA_SUPPORTED` class attribute.
1. For non-wheel (custom) checks, unless `disable_py3_validation` is set, a background goroutine emits the `datadog.agent.check_ready` recurrent series — a relic of the Python 2→3 migration linter that today just reports `status:python3` without linting anything.
1. Wrap the class in a [`PythonCheck`](<<<SRC>>>/pkg/collector/python/check.go) whose `Configure` parses the common instance fields (`min_collection_interval`, `empty_default_hostname`, `service`, `no_index`, and the Python-only `run_once`, which schedules the check as a one-shot by forcing `Interval() == 0`).
1. `Configure` then instantiates the Python check object through `get_check`; if the constructor fails, it retries once through `get_check_deprecated`, the legacy signature that passes the whole agent configuration to the constructor (deprecated, logged as a warning).

A Python check can decline a configuration the same way a Go check does with `ErrSkipCheckInstance`: it raises an error whose message contains a fixed pattern (`"The integration refused to load the check configuration..."`), which [`check.go`](<<<SRC>>>/pkg/collector/python/check.go) string-matches and converts to `ErrSkipCheckInstance`. The pattern must stay in sync with `SkipInstanceError` in integrations-core. This is how a config falls through between the Go and Python implementations of dual-implementation integrations (see [Go core checks](corechecks.md)).

## GIL discipline

Every entry into the interpreter goes through a [`stickyLock`](<<<SRC>>>/pkg/collector/python/helpers.go): `runtime.LockOSThread()` (CPython requires a stable OS thread) followed by rtloader's `ensure_gil`. `PythonCheck.Run` holds the lock for the *entire* run: acquire, `run_check`, commit the sender, collect warnings, release.

The practical consequence: Python checks serialize on the GIL no matter how many [runner workers](collector.md) exist. Multiple workers still help because well-behaved checks release the GIL during I/O (network calls, subprocesses), but CPU-bound Python checks starve each other. `Cancel`, `GetDiagnoses`, and even the garbage-collection finalizer must each take the sticky lock too.

## Builtin modules and Go callbacks

| Python module | Functions (selection) | Go side |
|---|---|---|
| `aggregator` | `submit_metric`, `submit_service_check`, `submit_event`, `submit_histogram_bucket`, `submit_event_platform_event` | [`pkg/collector/aggregator/aggregator.go`](<<<SRC>>>/pkg/collector/aggregator/aggregator.go); routed to the right `Sender` by check ID via the global [`CheckContext`](<<<SRC>>>/pkg/collector/aggregator/check_context.go) |
| `datadog_agent` | `get_config`, `get_hostname`, `get_clustername`, `get_host_tags`, `headers`, `send_log`, `set_check_metadata`, `set_external_tags`, `read/write_persistent_cache`, `obfuscate_sql`, `obfuscate_sql_exec_plan`, `obfuscate_mongodb_string`, `emit_agent_telemetry`, `report_issue`/`resolve_issue` | [`pkg/collector/python/datadog_agent.go`](<<<SRC>>>/pkg/collector/python/datadog_agent.go), [`health_platform.go`](<<<SRC>>>/pkg/collector/python/health_platform.go) |
| `tagger` | `tag` | [`pkg/collector/python/tagger.go`](<<<SRC>>>/pkg/collector/python/tagger.go) — queries the [tagger](../containers/tagger.md) |
| `containers` | `is_excluded` | [`pkg/collector/python/containers.go`](<<<SRC>>>/pkg/collector/python/containers.go) — workload filter |
| `kubeutil` | `get_connection_info` | [`pkg/collector/python/kubeutil.go`](<<<SRC>>>/pkg/collector/python/kubeutil.go) — kubelet connection info |
| `_util` | `get_subprocess_output` | [`pkg/collector/python/util.go`](<<<SRC>>>/pkg/collector/python/util.go) / [`subprocesses.go`](<<<SRC>>>/pkg/collector/python/subprocesses.go) — subprocesses run on the Go side and are tracked so they can be terminated at shutdown |
| `util` | `headers` (legacy alias) | same as `datadog_agent.headers` |

The metric data path is therefore: Python `AgentCheck.gauge(...)` → C builtin `aggregator.submit_metric` → Go `SubmitMetric` → `CheckContext.senderManager.GetSender(checkID)` → [aggregator](../pipelines/metrics/aggregation.md). No serialization crosses the boundary — arguments are passed as C types.

## PythonCheck lifecycle

1. **Configure**: builds the check ID, instantiates the Python object (owning interpreter references), applies common options to the sender.
1. **Run**: under the sticky lock, `run_check` invokes the instance's `run()` method; a non-empty returned string becomes the check error. The sender is committed and warnings are drained via `get_checks_warnings`. Runs are labeled with `pprof` labels (`check_id`) for profiling.
1. **Cancel**: called on unschedule; invokes `cancel_check` on the instance (releasing check-owned resources Python-side) and marks the Go object cancelled so a queued run becomes a no-op.
1. **Finalizer**: a Go GC finalizer decrefs the class and instance references from a goroutine (it must wait for the GIL), so Python-side memory for unscheduled checks is reclaimed *eventually*, not promptly.

## Wheels and integrations-core

Bundled integrations are wheels in the `datadog_checks.*` namespace, built from [DataDog/integrations-core](https://github.com/DataDog/integrations-core) at the commit pinned as `INTEGRATIONS_CORE_VERSION` in [`release.json`](<<<SRC>>>/release.json). Bazel builds them through [`deps/agent_integrations`](<<<SRC>>>/deps/agent_integrations); the legacy omnibus pipeline does the equivalent (see [Packaging](../deployment/packaging.md)). A `requirements-agent-release.txt` manifest of shipped versions is installed alongside the agent and used for minimum-version enforcement.

`agent integration install datadog-<name>==X.Y.Z` ([`command.go`](<<<SRC>>>/cmd/agent/subcommands/integrations/command.go)) lets users upgrade individual integrations between agent releases:

1. Refuses versions older than the shipped one and refuses `datadog-checks-base` entirely.
1. Downloads the wheel with `python -m datadog_checks.downloader`, which performs TUF/in-toto supply-chain verification (`--third-party` for community integrations; `--unsafe-disable-verification` as an escape hatch).
1. Installs with the embedded pip using `--no-index --no-deps` — dependencies always come from the agent's own environment.
1. `-w/--local-wheel` installs a local wheel after verifying it depends on `datadog-checks-base`.
1. Copies the wheel's bundled `conf.yaml.example` into `confd_path/<check>.d/`.

Custom checks are plain `.py` files in `additional_checksd` (default `/etc/datadog-agent/checks.d`), optionally with a config in `conf.d/<name>.d/conf.yaml`; they need no wheel and are loaded by the bare-module fallback.

## Memory tracking and debugging

1. `memtrack_enabled` routes rtloader's allocator through a Go-side tracker ([`memory.go`](<<<SRC>>>/pkg/collector/python/memory.go)), catching C-level leaks in the shim.
1. `telemetry.python_memory` (with telemetry enabled) samples interpreter allocation stats into the `pymem.alloc` / `pymem.inuse` telemetry series.
1. `tracemalloc_debug: true` enables CPython's tracemalloc for per-check memory diffs — and forces `check_runners: 1`, serializing all checks.
1. `agent check <name> --profile-memory` uses these hooks from the CLI; see [Diagnostics and CLI tools](../operations/diagnostics.md).

## Configuration

| Key | Default | Effect |
|---|---|---|
| `python_lazy_loading` | true | Initialize the interpreter on first Python check load instead of at startup |
| `additional_checksd` | `/etc/datadog-agent/checks.d` | Custom checks directory |
| `disable_py3_validation` | false | Skip the py3 linter for custom checks |
| `allow_python_path_heuristics_failure` | false | Tolerate interpreter path resolution failures (dev environments) |
| `memtrack_enabled` | false | Track rtloader allocations |
| `telemetry.python_memory` | true | `pymem.*` telemetry series |
| `tracemalloc_debug` | false | CPython tracemalloc; forces one check runner |
| `win_skip_com_init` | false | Skip COM init around check loading on Windows |
| `procfs_path` | — | Forwarded to psutil in checks via `get_config` |
| `ha_agent.enabled` | false | Makes the loader read `HA_SUPPORTED` from check classes |

## Platform and deployment notes

1. Host installs (Linux/macOS) embed Python at `/opt/datadog-agent/embedded`; Windows uses `embedded3` next to the binary, with conf.d under `%ProgramData%\Datadog`.
1. The standard container images ship Python and the full wheel set; slim variants built without the `python` tag (standalone DogStatsD, some OTel builds) have neither the loader nor the interpreter.
1. The Cluster Agent is built without Python by default — its check catalog is Go-only (see [Go core checks](corechecks.md)).
1. On AIX, cgo requires the rtloader shared library wrapped in a `.a` archive; the link flags in [`init.go`](<<<SRC>>>/pkg/collector/python/init.go) differ per platform.

## Gotchas

1. All Python checks share one interpreter and one GIL: a single check blocking in native code without releasing the GIL stalls every other Python check.
1. The `ErrSkipCheckInstance` bridge is a *string match* on the exception message; changing the message in either repository silently breaks Go/Python loader fallthrough.
1. Leaked Python references surface late because finalizer-driven decref needs the GIL; use `memtrack_enabled` plus `pymem.*` telemetry when hunting leaks.
1. Subprocesses spawned via `get_subprocess_output` execute on the Go side and are force-terminated at agent shutdown — Python-side cleanup code does not get a chance to run for them.
1. There is no guarantee a `pip` binary is on `PATH`; `agent integration` always resolves the embedded interpreter relative to the agent binary.
1. `run_once: true` in an instance makes the check one-shot (`Interval() == 0`), which also means it pins a dedicated runner worker while it runs.
1. Loading is serialized per-check by design: check *loading* also takes the sticky lock, so a slow import (heavy module) delays scheduling of other Python checks.
