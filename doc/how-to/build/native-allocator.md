# Linux native allocator

Linux Cgo product executables link the pinned jemalloc 5.3.0 PIC archive at their
final link. This does not make the executable fully static. Activation needs
neither `LD_PRELOAD` nor `MALLOC_CONF`. Existing statistics, profiling, fill,
debug, and tuning settings are unchanged.

## Build and distribution paths

`deps/jemalloc.BUILD.bazel` builds the shared compatibility library and the
static archive. `static_archive` selects only `libjemalloc_pic.a` from the foreign
build's output groups. `jemalloc_static` explicitly roots the public C API object:
rules_go 0.64.2 ignores `LibraryToLink.alwayslink`. Retaining the whole archive
would also retain jemalloc's optional C++ new/delete operators and impose a new
libstdc++ dependency on otherwise C-only products. libstdc++'s ordinary allocation
operators use the exported malloc/free implementation instead.

Rust final executables consume the same `CcInfo` through the pinned rules_rs
0.1.3 `link_deps` API. Rust passes `-nodefaultlibs` and places its libc before
native link options, so the allocator dependency repeats `-lc` after the archive.
This resolves stack-protector references through libc's linker script on arm64.
It also supplies GCC's outlined atomic helpers from `libgcc.a`; Rust's LLVM
compiler builtins and `-lgcc_s` alone do not provide those helpers.

`dd_agent_go_binary` opts in individual products, uses target-platform selects,
and keeps one external-linker option containing the export map and NVML lazy
binding. Core Agent's embedded main library already owns Cgo; other opted-in
binaries receive a Cgo source at the binary level. The allocator is never a
dependency of rtloader, libpython, or an integration DSO.

Omnibus currently invokes component build tasks in `datadog-agent.rb`,
`datadog-iot-agent.rb`, and other product recipes. Those tasks explicitly request
the allocator in `go_build`. The helper builds declared Bazel archive targets
for the target architecture, then supplies absolute archive paths only to the
final Go external link. It does not add allocator flags to `CGO_LDFLAGS`.
`CGO_ENABLED=0` and non-Linux targets do not invoke allocator builds.

The existing deb/rpm and Heroku package rules consume these executables. The
Dockerfiles consume package artifacts and retain `/opt/lib/nosys.so`. Neither
service environment files nor installation-script settings are needed for
activation. Archive filegroups are build inputs, without runtime package mappings.
The jemalloc package metadata and BSD license remain on the dependency graph;
license collection walks all dependency attributes, including linker inputs.

## Coverage and exclusions

| Product/variant | Intended coverage |
| --- | --- |
| Core Agent, standard/FIPS/Heroku | Cgo, rtloader, CPython RAW allocations, libc allocation calls from native checks |
| System Probe | Cgo, libpcap, native/Rust libraries linked into the executable, and loaded libc-using libraries |
| Process, Security, Trace, Private Action Runner | Native allocations in each executable; Python where enabled |
| DogStatsD, OTel, IoT, Cluster Agent/Cloud Foundry, trace-loader, CWS instrumentation, Host Profiler | Linux Cgo builds opt in; custom pure-Go builds remain pure |
| Installer, secret connector | FIPS Cgo builds opt in; portable non-FIPS pure-Go release builds remain unchanged |
| system-probe-lite, dd-procmgr, dd-procmgrd, par-control | Linux final Rust executable links use the same allocator/export contract; the discovery static library consumed by System Probe does not embed an allocator |
| rshell | Excluded: `AllThreadsSyscall` requires its explicitly pure-Go build |
| Agent Data Plane | Separately built Saluki artifact; its private Rust allocator is outside this change |
| Python interpreter executable, curl, JMX/JVM, build tools | Separate third-party executables, not Agent executable allocation paths |
| sbomgen, config-stream-client, disco, test fixtures | Development/build tools, not executables installed by these Agent packages |
| serverless-init | Separate serverless product; not consumed by the Agent/Heroku package or Omnibus paths inspected here. Its separate release pipeline is outside this change |
| AI usage native host, systray | macOS/Windows-only products, unchanged |
| macOS, Windows, AIX/other non-Linux targets | No jemalloc build/link dependency or allocator export flags |

The main release architectures are amd64 and arm64. Target selection is independent
of the build host. A target C toolchain must exist for cross compilation; setting
`GOARCH` or a Bazel platform alone does not install one. `--no-glibc` on the Agent
and System Probe tasks removes NVML, not Cgo or all libc calls. Such native builds
still need an allocator built with a compatible target libc toolchain. Pure-Go
variants skip the allocator. Fully static native variants keep their existing
static-link mode, with jemalloc as their native allocator.

The Go heap remains managed by Go. Python's pymalloc small-object pools and arena
behavior remain unchanged. Direct mmap, explicitly private/prefixed allocators,
separate linker namespaces, and libraries binding private libc entry points are
outside the coverage contract.

## Dynamic symbols and compatibility

`datadog-agent.map` uses an anonymous version script. Public allocator definitions
must be unversioned so existing GLIBC-versioned references can resolve to them.
It exports the allocation family, including `malloc_usable_size`, and jemalloc's
public extended/control APIs; unrelated symbols remain hidden, except existing
NVML exports. Small executable-only adapters provide `pvalloc` and `reallocarray`,
which jemalloc 5.3.0 does not define. In particular, libc's private pvalloc path
must not return a pointer later freed by jemalloc.

The packaged `libjemalloc.so` is retained because existing deployment environment
files and workloads preload it, and other processes can still use it. The
executable's public definitions take precedence over preload libraries. Tests
verify allocation and `mallctl` ownership with both preload orders and nosys.
Code must query the process-wide allocator (`RTLD_DEFAULT`), not explicitly open
a second allocator and use handle-local `dlsym` or `RTLD_NEXT` to mix its APIs with
process-wide pointers. There is no such explicit jemalloc loader in the Agent
paths inspected here.

Removing `LD_PRELOAD`, setting `MALLOC_CONF`, or preloading libc cannot restore
libc allocation in these executables. Rollback requires the previous executable
or a rebuild changing final linkage. Retaining the shared library also preserves
preload-based rollback behavior for older executables.

## Regression checks

```sh
dda inv invoke-unit-tests.run --tests=native_allocator --directory=tasks/unit_tests/libs/build
dda inv invoke-unit-tests.run --tests=go,utils --directory=tasks/unit_tests/libs/common
bazel test //test/native_allocator:bindings_test --test_output=errors
python3 test/native_allocator/verify_platforms.py --platform windows_x86_64
# Run macOS analysis on a macOS host (runtime patcher toolchain requirement):
python3 test/native_allocator/verify_platforms.py --platform macos_arm64 --platform macos_x86_64
```

The Bazel runtime test uses real Core Agent, System Probe, and Trace Agent
executables. Its DSOs are compiled against libc, without jemalloc. It checks
dynamic definitions, absence of a jemalloc `DT_NEEDED`, GLIBC-versioned DSO
references, executable symbol ownership, jemalloc arena ownership of pointers,
cross-library allocation/reallocation/free, zeroing, content preservation,
alignment, usable size, overflow, both dlopen visibility modes, legacy preloads,
nosys coexistence, and stripped copies. Both Linux CI architecture jobs include
the test. Separate configured-action checks cover Windows cross builds, native
macOS configurations, and Linux pure-Go selection.

Run `test/native_allocator/verify.py` against installed artifacts with `--binary`,
`--driver`, `--library`, `--jemalloc`, and `--nosys` paths. The test driver preload
is a probe, not an allocator. The first run has no jemalloc preload and every run
unsets `MALLOC_CONF`.

For embedded Python, install `test/native_allocator/allocator_check.py` in
`checks.d`, create `conf.d/allocator_check.d/conf.yaml` with `instances: [{}]`, set
`telemetry.python_memory: true`, and run `agent check allocator_check`. Set
`DD_ALLOCATOR_TEST_LIBRARY` to the libc-built fixture. This exercises the actual
embedded interpreter and rtloader RAW hooks. Native integration checks and
service startup/shutdown must additionally be tested using the packaged runtime.

`library.so` also implements the actual shared-library integration ABI. Its
`Run` entry point checks arena ownership of the three strings allocated by
`C.CString` in the Agent, then exercises native allocation and reports
`allocator.ffi.success`. This tests the real `RTLD_LOCAL` loader, including its
ownership and permission checks.

For an installed package or container, run the reproducible startup/check/shutdown
smoke test below. It uses temporary configuration, enables Python RAW accounting,
runs both allocator checks and the disk integration, and requires graceful exit.
The calling user must satisfy the normal native integration ownership policy
(root or the Agent user). Keep the image's nosys preload in container runs:

```sh
python3 test/native_allocator/verify_installed.py \
  --agent /opt/datadog-agent/bin/agent/agent \
  --library /path/to/library.so \
  --nosys /opt/lib/nosys.so --integration disk \
  --trace-agent /opt/datadog-agent/embedded/bin/trace-agent \
  --system-probe /opt/datadog-agent/embedded/bin/system-probe
```

Artifact linkage tests are not full telemetry E2E or production validation.
