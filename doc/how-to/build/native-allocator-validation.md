# Static native allocator validation, 2026-10-22

The implementation is in the worktree. Validation below establishes allocator
ownership in real product executables; it does not establish complete release,
FIPS certification, kernel-feature, or production E2E coverage.

See [native-allocator.md](native-allocator.md) for the complete intended product
matrix, build-path rationale, exclusions, compatibility, and rollback behavior.

## Verified artifacts

| Platform / flavor | Build and runtime evidence |
| --- | --- |
| Linux arm64 standard | Core Agent, System Probe, and Trace Agent built; 30 binding cases passed, including release `-s -w`, separately stripped copies, both loader visibility modes, legacy jemalloc preloads, and nosys |
| Linux arm64 Debian | Actual package built and installed in a disposable Linux container; installed Core started, answered status, and exited cleanly with `LD_PRELOAD`, `MALLOC_CONF`, and `LD_LIBRARY_PATH` unset |
| Linux arm64 packaged components | Installed Core, Trace, Process, Security, Private Action Runner, dd-procmgr, and par-control passed binding tests, including stripped copies and both legacy jemalloc preload orders |
| Linux arm64 Rust | dd-procmgr, dd-procmgrd, system-probe-lite, and par-control built; dd-procmgr and par-control additionally passed runtime binding tests |
| Linux arm64 FIPS | Trace, installer, and secret connector built with Microsoft Go and FIPS tags; all three passed ELF export inspection and the allocator probe during `--help` with no jemalloc preload or MALLOC_CONF |
| Linux arm64 Heroku | Core, Trace, and Process Heroku executable targets built; full Heroku package/runtime unverified |
| Linux arm64 Invoke | `dda inv trace-agent.build` built the executable through the hybrid path; six binding cases passed, including stripping and both legacy preload orders |
| Linux amd64 standard | Trace Agent built and passed ten runtime binding cases under x86 emulation; Core/System Probe amd64 builds and native amd64 runtime were not completed locally |
| macOS arm64 | Trace Agent built and its version command ran; configured dependency and link-action checks passed |
| macOS amd64 | Trace Agent cross-built; configured dependency and link-action checks passed; runtime not exercised |
| Linux host targeting Windows amd64 | Core/System Probe/Trace configured dependency and GoLink action inspection passed with no jemalloc dependency or allocator export flags; full Windows compilation/runtime unverified |
| Linux pure-Go | Configured dependency inspection passed; explicit `CGO_ENABLED=0` helper tests passed; no full pure-Go product build in this run |
| IoT, minimal, no-glibc, fully static native, other opted-in products | Source paths and target gates assessed; no dedicated artifact/runtime validation for every variant |

## Native ownership evidence

The test DSOs are built against libc and contain GLIBC-versioned allocation
references. The probe verifies that the executable defines the standard and
extended allocation APIs, then uses `arenas.lookup` on actual pointers. It
exercises cross-library allocation, resize, content preservation, zeroing,
alignment, usable size, overflow, and free through both RTLD_LOCAL and RTLD_GLOBAL.
The executable has no `DT_NEEDED` on jemalloc.

Dynamic-loader diagnostics from the running arm64 Core Agent showed:

```text
libdatadog-agent-rtloader.so -> /opt/datadog-agent/bin/agent/agent: malloc, free, malloc_usable_size [GLIBC_2.17]
libpython3.13.so -> /opt/datadog-agent/bin/agent/agent: malloc, calloc, realloc, free [GLIBC_2.17]
libdatadog-agent-allocator_ffi.so -> /opt/datadog-agent/bin/agent/agent: malloc, calloc, realloc, free, malloc_usable_size [GLIBC_2.17]
```

The installed Python check verified rtloader's RAW allocator hooks, RAW accounting,
pointer ownership, and ownership of the C++ `Three` object. Python's small-object
hook still pointed into libpython. The actual native check loader passed its
permission checks and called a fixture which verified ownership of Cgo-allocated
`C.CString` arguments. Both checks emitted their success metric. The disk check
also ran successfully.

The container smoke used updated Core/System Probe/Trace binaries and their
matching embedded Python runtime overlaid onto an existing Agent image. It kept
`/opt/lib/nosys.so` preloaded and removed MALLOC_CONF. Core, Trace, and System Probe
started and stopped cleanly. Raw Bazel outputs needed `LD_LIBRARY_PATH` for this
overlay; the installed Debian Core did not. This was not a rebuilt release image,
and the container's system-probe-lite helper was the original image version.

## Commands and checks

Linux builds used `--config=no-remote-cache --sandbox_base=/tmp/allocator-sandbox`.
Arm64 ran natively in Colima; amd64 used an amd64 development image under emulation.

```sh
bazel build //cmd/agent //cmd/system-probe //cmd/trace-agent
bazel test //test/native_allocator:bindings_test
bazel test //test/native_allocator:bindings_test --//:release=true \
  --@rules_go//go/config:gc_linkopts=-v --jobs=4
bazel build //packages/agent/linux:debian //pkg/procmgr/rust:dd-procmgr \
  //pkg/procmgr/rust:dd-procmgrd //pkg/discovery/module/rust:system-probe-lite \
  //pkg/privateactionrunner/par-control:par-control --jobs=8
bazel build //cmd/trace-agent //cmd/installer //cmd/secret-generic-connector \
  --//packages/agent:flavor=fips --@rules_go//go/toolchain:sdk_name=msgo --jobs=8
python3 test/native_allocator/verify_platforms.py --platform windows_x86_64
python3 test/native_allocator/verify_platforms.py --platform macos_arm64 --platform macos_x86_64
dda inv trace-agent.build
dda inv invoke-unit-tests.run --tests=native_allocator --directory=tasks/unit_tests/libs/build
dda inv invoke-unit-tests.run --tests=go,utils --directory=tasks/unit_tests/libs/common
```

`verify.py` was also run with explicit `--binary`, `--driver`, `--library`,
`--jemalloc`, and, for container cases, `--nosys` artifact paths. Security Agent,
Private Action Runner, and Rust CLI tests used `--version-arg=--help`.
`verify_installed.py` ran with Core, Trace, System Probe, nosys, the native fixture,
and `--integration disk`, as documented in the allocator guide.

The verbose release build captured the actual final external links. They selected
`libjemalloc_pic.a` explicitly, retained the public C API object with `-u` roots,
and included the anonymous export map, export-dynamic, and lazy binding. Core's
rtloader RPATH remained present. No `-ljemalloc` selection was used.

The package contained `embedded/lib/libjemalloc.so.2`, its compatibility symlink,
and `LICENSES/jemalloc-COPYING`. It contained no jemalloc/compat build archive.

Focused Linux Invoke tests passed earlier in the run (8 allocator, 11 Go-helper,
23 utility tests). Final macOS runs passed the allocator and Go-helper tests;
four existing utility tests failed because their Xcode command mock returns a
Mock rather than a version string. The new pure-Go cross-build test passed.
New Python files passed Ruff lint/format; changed Bazel files passed Buildifier
after formatting; release notes passed RST and unique-ID checks; whitespace checks
passed. The PR-existence release-note check skipped because there is no PR.

## Remaining validation and environment limits

- Full Omnibus deb/rpm release assembly, RPM installation, installation-script
  deployment, and systemd service operation on a dedicated host remain unverified.
- The current Bazel Debian recipe omits integration wheels, post-install Python
  scripts, and System Probe. `dpkg` completed, but its post-install helper reported
  missing `python-scripts/post.py`. Package startup passed; this is not a clean
  full-release installation result. The separate container exercised Python and
  native integration behavior with its existing integrations.
- Full FIPS service/provider startup, Core/System Probe FIPS, Heroku runtime,
  every minimal/static/native variant, and native amd64 runtime remain unverified.
- No live-kernel System Probe validation, full telemetry E2E, performance/RSS
  regression benchmark, or production rollout was performed.
- Initial package builds exhausted the disposable VM disk. Later concurrent
  build reruns hit file-descriptor limits. Native release tests passed after
  reducing concurrency and raising the test-owned Bazel server's limit. The final
  amd64 rebuild retry stopped during dependency loading with that limit; its
  earlier ten-case runtime result precedes the final Rust-specific `-lgcc` addition.
- CI configuration includes the binding test in both existing Linux architecture
  jobs and adds cross-target inspection. Remote CI has not run for this worktree.

The shared allocator remains packaged for existing preload settings. Runtime
tests establish executable ownership under those settings. Disabling jemalloc
now requires rolling back or rebuilding the executable; deleting LD_PRELOAD is
not an allocator rollback.
