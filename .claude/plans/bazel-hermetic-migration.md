# Bazel Hermetic Migration Plan

Migrate the agent build from invoke/omnibus to a fully hermetic Bazel build, retiring the Nix dev-shell effort.

This plan is written against the repo **as it actually is** on `nick/nix-investigation`, not against the
prompt's stale model. Several "gaps" assumed by the task brief are already closed. Read the reality-delta table
first.

> **CLI wrapper:** All Bazel invocations use the repo's `bzl` wrapper, not `bazel` directly.
> Everywhere this plan writes `bazel <subcommand>`, run it as `bzl <subcommand>`.
> Example: `bzl build //cmd/agent --platforms=//bazel/platforms:linux_x86_64`

---

## POC results & updated path (2026-06-10)

A POC run (workflow `bazel-full-migration.js`, on `nick/bazel-investigation`, target platform
`//bazel/platforms:linux_arm64` — this host is aarch64) validated the laddered per-binary approach below.
**The thesis is proven:** a non-trivial binary builds fully under Bazel, the two scariest unknowns came back
GO with reusable patterns, and the full agent's remaining blockers are now a short, concrete list.

### Status snapshot

| Target | State | Evidence |
|---|---|---|
| `//cmd/trace-agent` | ✅ **fully Bazel-built, end-to-end** | warm-cache build (758/762 action-cache hits, ~3s), `version` runs, `trace-agent_arm64_floor` test PASSES |
| `//cmd/process-agent` | ✅ built green | compiled + ran at verify time; `--nobuild` resolves |
| `//cmd/system-probe` | ✅ built green | compiled + ran (eBPF tier); `--nobuild` resolves |
| `//cmd/security-agent` | ✅ built green | compiled + ran (eBPF tier); `--nobuild` resolves |
| `//cmd/cluster-agent` | ⏸ blocked | godror/oracle (see below) |
| `//cmd/agent` (full) | ⏸ blocked | godror/oracle, then `pkg/gpu/testutil` test-tag (see below) |

Note: `bazel-bin` is re-pointed across configurations, so artifacts from one binary get evicted when another
builds. Re-confirm any green binary with `bzl build --nobuild <t> --platforms=//bazel/platforms:linux_arm64`
(0-action "Build completed successfully" = closure fully wired) and rebuild on demand — the warm cache makes
that cheap (trace-agent re-link was ~3s).

### Spike outcomes

- **eBPF `//go:embed`: GO** (primary go/no-go for the eBPF tier). A Go package that embeds a Bazel-built
  eBPF `.o` compiles end-to-end.
- **Version stamping: GO.** rules_go `x_defs` + a `--workspace_status_command` script reproduce omnibus's
  exact version format.
- **glibc floor: PARTIAL.** arm64 floor green (libpython 2.17 ≤ 2.23). Gap: no `glibc_floor_test` is
  instantiated for `@cpython//:python_unix_shared` yet, and x86_64 must be verified on an x86_64 CI runner
  (it can't resolve toolchains on this aarch64 host).

### Proven patterns (the reusable how-tos — use these for every subsequent binary)

1. **Gazelle tag-union (mandatory).** Run gazelle once with the **union of all flavor tags**
   (`bzl run //:gazelle -- update -build_tags "<union>" ./pkg ./comp ./cmd`; canonical string established in
   the POC and noted in `bazel/AGENTS.md`). rules_go does per-file `//go:build` selection at *build* time, so
   one BUILD file must carry every variant. An untagged run drops variants → phantom `undefined: X` errors
   (e.g. `no_otlp.go` selected instead of `pipeline.go`).
2. **`gotags` on `go_binary` is VALID — uncertainty resolved.** The Phase 1 caveat below (lines questioning
   whether `gotags` works on `go_binary`, with a `go_cross_binary`/global-flag fallback) is **settled**:
   `cmd/agent/BUILD.bazel` carries `gotags=[...]+select(...)` and rules_go honors it (`-tags …` in the
   GoCompilePkg action). **The fallback branch is unnecessary — delete it when revising Phase 1.**
3. **eBPF embed wiring.** `copy_file` the eBPF `.o` into the **consuming** Go package's own directory, then
   reference the copy target in `embedsrcs`. A direct cross-package label
   (`embedsrcs=["//other:thing.o"]`) does **not** work — `//go:embed` resolves relative to the consuming
   package dir, so the `.o` must land there. For the `asset_reader_bindata_*.go` pattern, replace the
   committed `build/<arch>/*.o` blobs with `copy_file` from the corresponding eBPF targets.
4. **Convergent build-file loop** (resolve a target's graph in ~3–5 batches, not one package at a time):
   gazelle(tag-union) → `bzl build --nobuild <t>` → parse `no such package`/`no such target` → batch-unlock
   the blocking `gazelle:ignore` (own/parent BUILD) or root `gazelle:exclude` → repeat. Editing the **root**
   `BUILD.bazel` excludes also auto-updates `@bazelify_go_work//:go.work` (blast radius — re-confirm the
   prebuilt path still resolves after). Edit it with Edit/python, **never `sed`** (path-slash delimiter
   collisions corrupt the file).

### Blocker inventory (what stands between here and the full agent)

| Blocker | Affects | Nature | Action |
|---|---|---|---|
| **godror / `oracle` tag** | cluster-agent, full agent | cgo Oracle driver needs the proprietary Oracle Instant Client SDK (`dpiImpl.h`) — absent in a hermetic sandbox | **Dedicated external-module bring-up** (a Bazel module vendoring/wrapping the SDK), same category as the eBPF pipeline. Not an inline fix. |
| **`pkg/gpu/testutil` test-tag** | full agent (next blocker after oracle) | `mocks.go` missing the `test` build tag → `undefined: core.MockBundle / fxutil.Test / telemetry.Mock` | Small source/BUILD fix (add the tag or adjust gazelle stamping). |
| **eBPF `.o` embeds at scale** | system-probe, security-agent (and agent's closure) | the `runtime-security.o` / `usm-debug.o` class of committed blobs | Apply pattern #3 above to each `//go:embed` site, wiring `copy_file` from the eBPF targets. |
| **glibc floor coverage** | CI gate | no floor test for libpython; x86_64 unverified | Add two `glibc_floor_test` entries for `@cpython//:python_unix_shared` (x86_64=2.17, arm64=2.23); they ride the existing `bazel:test:linux-{amd64,arm64}` jobs via `//...`. Source `objdump` from the hermetic toolchain. |
| **packaging cutover** | per-binary | flip `dda_built_*` sources from `@X_binary//:X` to `//cmd/X` once green | Do it per green binary; the POC's P7 pass stalled, so trace-agent's flip needs re-applying. |

### Updated path forward

The migration is a **laddered per-binary cutover over the `prebuilt_file` hybrid** (each binary flips
independently once green; full-agent compilation is NOT required for the packaging build to work):

1. **Land the proven scaffolding** as reviewable commits: the eBPF-embed `copy_file` pattern (replace the
   committed `.o` blobs), the stamping `workspace_status.sh` + `x_defs`, the libpython floor tests, and the
   canonical-tag note. Remove the throwaway spike package `pkg/ebpf/spike_ebpf_embed/`.
2. **Finish the no-eBPF tier**: trace-agent (done — re-apply packaging flip), process-agent (done),
   cluster-agent (blocked only on oracle — ships once godror is handled or the `oracle` check is excluded).
3. **eBPF tier**: system-probe, security-agent build green; wire their real eBPF embeds via pattern #3 to
   remove the committed-blob dependency.
4. **godror/oracle external module** (parallel workstream): unblocks cluster-agent + the full agent.
5. **Full agent**: with oracle handled and the `gpu/testutil` tag fixed, drive `//cmd/agent` to green
   (closure already resolves; it's a finite list of compile fixes, not an unknown).
6. **Per-binary packaging cutover + CI floor gate**, then omnibus retirement (Phases 4–6 below).

Detailed findings live in the agent memory note `bazel-full-migration-poc`. The phase-by-phase detail below
remains the reference for the mechanics; this section supersedes its optimistic framing of Phase 1.

---

## Reality delta (corrections to the task brief's assumptions)

| Brief assumed | Actual state (verified) | Source |
|---|---|---|
| `@cpython` is a **downloaded pre-built** Python; verify glibc floor or cross-compile from scratch | CPython **3.13.13** is **built from source** via `configure_make` (rules_foreign_cc) under the registered hermetic GCC toolchains. Architecture exists; only the floor is unverified. | `deps/cpython.MODULE.bazel`, `deps/cpython.BUILD.bazel:169` |
| Embedded Python is 3.12 / `libpython3.12.so` | It is **3.13** / `libpython3.13.so.1.0` (+ `lib/libpython3.so` stable-ABI symlink) | `deps/cpython.BUILD.bazel:158,246,418` |
| rtloader must be migrated from CMake (`cmake()` vs rewrite as `cc_library`) | Already **pure Bazel**: `cc_library`/`cc_shared_library` linking `@cpython`. CMake survives only for the legacy `dda inv rtloader.make` path. | `rtloader/BUILD.bazel` |
| omnibus still produces deb/rpm; `rules_pkg` only "exists" | deb/rpm **already produced** by `rules_pkg` in `packages/agent/linux` consuming hybrid `prebuilt_file` binaries; the installer package migration is mid-flight with a documented methodology | `packages/agent/linux/BUILD.bazel`, `packages/installer/MIGRATION_PLAN.md` |
| libpcap is a "host assumption" to be declared | Already a declared Bazel dep | `deps/libpcap/libpcap.MODULE.bazel` |
| macOS deployment target needs a toolchain shim | Already set: `--macos_minimum_os=12.0` in `.bazelrc` | `.bazelrc:49` |
| Non-root install paths are hardcoded/omnibus-only | Already a build parameter: `//:install_dir` string_flag (default `/opt/datadog-agent`) | `BUILD.bazel:901` |
| The 10 agent binaries have no Bazel rules | True. They are **hybrid `prebuilt_file` repos** (`@agent_binary`, …) capturing `dda`-built ELFs, consumed by packaging as `dda_built_*_binary`. `cmd/agent/BUILD.bazel` is a `# gazelle:ignore` stub. | `MODULE.bazel:451-487`, `packages/agent/product/BUILD.bazel:133`, `cmd/agent/BUILD.bazel` |

**The true gap is two things:** (1) replacing the hybrid `prebuilt_file` agent binaries with real `go_binary`
rules carrying the correct build tags (Phase 1), and (2) a per-platform glibc-floor CI gate on both the agent
ELF and `libpython` (Phases 3 + 5). Phases 2 and 4 are mostly *finishing* and *deleting*, not *building*.

**Methodology:** follow the path the installer package migration already walked — `prebuilt_file` →
real Bazel target → CI file-tree/metadata comparison gate → fix-differences → cutover + delete legacy. Reuse
its vocabulary (`packages/installer/MIGRATION_PLAN.md`).

---

## Phase 1 — Agent binaries via `go_binary`

**Goal:** Replace the 10 hybrid `prebuilt_file` agent binaries with hermetic `go_binary` rules carrying the
correct per-binary/per-flavor build tags.

**Current state:**
- Binaries are captured as hybrid repos in `MODULE.bazel:451-487`: `@agent_binary`, `@installer_binary`,
  `@privateactionrunner_binary`, `@process_agent_binary`, `@trace_agent_binary`, `@trace_loader_binary` (the
  full set of 10 product binaries is not yet even captured — only these are).
- Packaging consumes them as `dda_built_agent_binary` etc. in `packages/agent/product/BUILD.bazel:133-165`.
- `tasks/agent.py:build()` (lines 68-200) computes tags via `compute_build_tags_for_flavor` /
  `get_default_build_tags` (`tasks/build_tags.py`) and shells to `go_build`. `enable_bazel=False` is the default
  and only swaps rtloader, never the agent ELF.
- Precedents that already work: `cmd/cws-instrumentation/BUILD.bazel`, `cmd/loader/BUILD.bazel`,
  `cmd/config-stream-client/BUILD.bazel` are real `go_binary` + Gazelle `go_library`.
- Tag-injection precedent: `bazel/rules/go_build_tags/_gazelle_extension.go` already injects `gotags=["test"]`
  onto every `go_test` via the custom `gazelle_binary`. This is the in-repo mechanism to extend.

**Binaries to convert (each gets a `cmd/<x>/BUILD.bazel` with `go_library` + `go_binary`):**
`agent`, `process-agent`, `trace-agent`, `system-probe`, `cluster-agent`, `installer`, `dogstatsd`,
`otel-agent`, `security-agent`, `cws-instrumentation` (already done — verify tags), plus the flavor/aux
binaries the product ships: `iot-agent`, `cluster-agent-cloudfoundry`, `trace-loader`,
`privateactionrunner`, `systray` (Windows). Confirm the authoritative list against
`omnibus/config/projects/agent.rb` + `packages/agent/product/BUILD.bazel` during step 1.

**Build-tag strategy (the crux):**

> **Verify first — the plan's most uncertain claim.** The strategy below assumes `gotags = [...]` is a valid
> attribute on `go_binary`. This is **proven only for `go_test`** (what `bazel/rules/go_build_tags` stamps);
> the one `go_binary` precedent (`cmd/cws-instrumentation`) uses a plain `select()` on deps with no `gotags`.
> Before implementing, confirm with `bzl query --output=build //cmd/cws-instrumentation:cws-instrumentation`
> or the rules_go rule def. **If `gotags` is absent on `go_binary`,** the mechanism shifts to the global
> `--@rules_go//go/config:tags` flag composed via `config_setting`+`select` (named `.bazelrc` configs), or a
> `go_cross_binary` with a tags transition — and step 4 changes accordingly (the Gazelle extension cannot stamp
> a non-existent attribute). The *intent* (per-binary tag sets) is achievable either way; only the wiring differs.

`tasks/build_tags.py` produces, per binary/flavor: a `bundle_<name>` tag, flavor include/exclude deltas
(`base`/`iot`/`heroku`/`fips`), and conditional tags (`ebpf_bindata`, `nvml` gated on `--glibc`). Map to
rules_go:
1. **Per-binary static tags** → `gotags = [...]` literal on each `go_binary` (e.g. `bundle_agent`). These never
   vary by configuration.
2. **Flavor selection** → a `//bazel/flags:flavor` `string_flag` (`base`/`iot`/`heroku`) + `config_setting`s,
   feeding a `select()` into `gotags` and into the `go_binary` `embed`/`deps` where flavor changes the
   dependency set. Reuse the existing FIPS axis — `//bazel/platforms:fips` constraint already exists
   (`bazel/platforms/BUILD.bazel:7`) and the `*_fips` platforms are defined; key FIPS tags off that, not a new flag.
3. **Global cross-cutting tags** (`ebpf_bindata`, `nvml`) → `--@rules_go//go/config:tags=` set in named
   `.bazelrc` configs (`build:ebpf_bindata`, etc.) so they compose with the per-target `gotags`.
4. **Avoid hand-maintaining tags in BUILD files.** Extend the `bazel/rules/go_build_tags` Gazelle extension to
   stamp `bundle_<name>` onto `go_binary`s under `cmd/`, the same way it stamps `gotags=["test"]` today. This
   keeps the tag sets generated, not hand-edited — consistent with the "Gazelle is the BUILD author" rule in
   `bazel/AGENTS.md`.

**Steps:**
1. Enumerate the authoritative binary + per-flavor tag matrix from `tasks/build_tags.py` and
   `packages/agent/product/BUILD.bazel`. Record it as a table in this plan's appendix.
2. `bzl run //:gazelle -- update ./cmd/agent` (and each remaining `cmd/<x>`) to generate `go_library`.
   Hand-add the `go_binary` where Gazelle can't infer it (it generates `_lib` but the binary target with
   `gotags`/`select` is manual or extension-stamped). `bzl run //bazel/buildifier`.
3. Add the flavor `string_flag` + `config_setting`s under `bazel/flags/` (or root `BUILD.bazel` alongside
   `//:release`). Add `build:ebpf_bindata` / `build:nvml` configs to `.bazelrc` (requires `@DataDog/agent-build`
   review — `.bazelrc` is gated).
4. Extend `bazel/rules/go_build_tags/_gazelle_extension.go` to stamp `bundle_<name>` `gotags` on `cmd/`
   `go_binary`s. Rebuild the `gazelle_binary`; re-run Gazelle.
5. Build each binary per platform: `bzl build //cmd/agent --platforms=//bazel/platforms:linux_x86_64`
   (and `linux_arm64`, `macos_*`, `windows_x86_64`). Fix C-signature / GCC-14-strictness breakages as they
   surface (the `noop_version` class of bug — see Phase 6).
6. Swap packaging from the hybrid repos to the real targets: in `packages/agent/product/BUILD.bazel` replace
   `@agent_binary//:agent` (etc.) with `//cmd/agent` in the `dda_built_*_binary` `pkg_files`. Rename those
   intermediates off the `dda_built_` prefix.
7. Add the dda bridge: `dda inv agent.build` shells to `bzl build //cmd/agent …` via
   `tasks/libs/build/bazel.py` (the helper `rtloader_install_with_bazel` already uses). Flip
   `enable_bazel=True` as the default. Keep the legacy `go_build` path reachable behind `--no-enable-bazel`
   only until cutover.

**Key files:** `cmd/*/BUILD.bazel` (new), `bazel/flags/BUILD.bazel`, root `BUILD.bazel`,
`bazel/rules/go_build_tags/_gazelle_extension.go`, `packages/agent/product/BUILD.bazel`, `MODULE.bazel`
(remove `prebuilt_file` blocks at cutover), `tasks/agent.py`, `.bazelrc`.

**Risks / open questions:**
- Some binaries have large, configuration-dependent dep graphs; `select()` proliferation can explode the
  configured-target count. Prefer flavor as a single flag axis over many per-tag transitions.
- FIPS, Windows `.syso` resource embedding (`build_rc`/`build_messagetable` in `tasks/agent.py:138-148`), and
  `serverless-init`/`otel-agent` may need bespoke handling — confirm each builds before cutover.
- Stamping: omnibus injects version/commit ldflags. Use rules_go `x_defs` + Bazel stamping
  (`bazel_lib//lib:stamping.bzl`) wired to `release.json` (already exposed via `@dd_release_json`).

**What disappears:** all `prebuilt_file(...)` agent-binary blocks in `MODULE.bazel`; the `dda_built_` aliasing
in `packages/agent/product`; the `go_build` call and tag-plumbing in `tasks/agent.py:build()`; the
`enable_bazel` flag (deleted at cutover); the Nix `nix develop` Go toolchain wiring.

---

## Phase 2 — rtloader via Bazel (finish + delete legacy)

**Goal:** Make the Bazel `cc_library` rtloader the only build path; delete the CMake path.

**Current state (already done):**
- `rtloader/BUILD.bazel` defines `cc_library(:rtloader)`, `cc_library(:rtloader_headers)`,
  `cc_shared_library(:datadog-agent-rtloader)`, and `dd_cc_packaged(:rtloader_pkg)`. The three/cpython backend
  links `@cpython//:python_unix_shared` / `@cpython//:python_pkg` (`rtloader/BUILD.bazel:179-203`). The
  `find_package(Python3)` CMake discovery question the brief raises is **moot** — cc_library won, pointed
  directly at `@cpython`.
- `tasks/rtloader.py` already has `install_with_bazel()` (lines 99-148) calling `//rtloader:install`,
  `@cpython//:install`, `//rtloader:install_python_env`, `//bazel/rules:replace_prefix`.
- `rtloader/three/CMakeLists.txt` retains the `DD_RTLOADER_PYTHON3_ROOT` Nix hint — used only by the legacy
  `dda inv rtloader.make` path.

**Steps:**
1. Confirm the Bazel `:datadog-agent-three` (three backend) target builds and links on all platforms; close any
   gap vs. the CMake target's source list (`rtloader/three/CMakeLists.txt:48-61`).
2. Route `dda inv rtloader.make` → `install_with_bazel` unconditionally; make `dda inv rtloader.clean` a no-op
   that points at `bazel` (never `bzl clean` — see `bazel/AGENTS.md`).
3. Delete `rtloader/**/CMakeLists.txt`, `tasks/rtloader.py:make/install` (CMake bodies), the
   `DD_RTLOADER_PYTHON3_ROOT` block in `rtloader/three/CMakeLists.txt`, and `cmake_options` plumbing in
   `tasks/agent.py`.

**Key files:** `rtloader/**/CMakeLists.txt` (delete), `tasks/rtloader.py`, `tasks/agent.py`,
`rtloader/three/CMakeLists.txt`.

**Risks:** the legacy path may still be referenced by AIX builds (`tasks/agent.py:114` special-cases AIX
rtloader). Keep AIX on the native prereq path; AIX is out of Bazel scope.

**What disappears:** all rtloader CMake files; the Nix `DD_RTLOADER_PYTHON3_ROOT` shim; CMake invoke bodies.

---

## Phase 3 — Embedded Python glibc floor (verify, then wire if needed)

**Goal:** Prove the Bazel-built `libpython` satisfies the floor (x86_64 ≤ 2.17, aarch64 ≤ 2.23); if not, wire
`configure_make` to the hermetic GCC toolchain (NOT a from-scratch rebuild).

**Current state:**
- `deps/cpython.BUILD.bazel:169` builds CPython via `configure_make` (rules_foreign_cc) producing
  `libpython3.13.so.1.0` + `lib/libpython3.so` (`:158,246`). It already sets `AR=@llvm_toolchain_llvm//:ar`
  and is `target_compatible_with` per-platform — i.e. it intends to run under the registered toolchains.
- This is exactly the failure mode that bit Nix: Nix built `libpython` against the store's glibc 2.42 so the
  bundled Python failed the floor even though the agent ELF passed. The open question is whether
  `configure_make` here consumes `gcc_toolchain_x86_64`/`aarch64` (glibc 2.17/2.23) or leaks host glibc.

**P3 status: arm64 COMPLETE (verified 2026-06-09, re-verified 2026-06-09 with absolute-path fix). x86_64 deferred to CI.**

**Root cause of prior verification failures ("libpython3*.so* not found under bazel-bin despite BUILD_EXIT:0"):**
The `find -L bazel-bin` command used a *relative* path. `bazel-bin` is a symlink that only resolves from the
repo root. The orchestration script ran `find` from a different working directory, so no files were found even
though the build artifact existed. Fix: always use `find -L "$REPO/bazel-bin"` with the absolute repo path.
See corrected commands in Step 1 below.

**P3 investigation findings (2026-06-09, aarch64 dev machine — arm64 PASSES; x86_64 deferred to CI):**

The `bzl build @cpython//:python_unix_shared --platforms=//bazel/platforms:linux_x86_64` invocation failed
at analysis time (0 actions executed) with:

```
No matching toolchains found for types: @@bazel_tools//tools/cpp:toolchain_type
```

Toolchain resolution debug (`--toolchain_resolution_debug='@bazel_tools//tools/cpp:toolchain_type'`)
showed:

1. `gcc_toolchain_x86_64` is **compatible with the target platform** (linux/x86_64) — it was NOT rejected
   for target constraints.
2. It **was rejected for the execution platform**: `Incompatible execution platform @@platforms//host:host;
   mismatching values: x86_64`.
3. Root cause: the machine is **aarch64** (`uname -m = aarch64`, kernel shows arm64 build). Bazel's
   `@@platforms//host:host` resolves to `[linux, aarch64]`. The `gcc_toolchain_x86_64` binary is a native
   x86_64 ELF (`file bin/x86_64-unknown-linux-gnu-gcc → ELF 64-bit LSB ... x86-64`; it fails to exec with
   `Could not open '/lib64/ld-linux-x86-64.so.2'`). So `exec_compatible_with = [linux, x86_64]` is correct
   and cannot be satisfied on this host. No registered toolchain can build for x86_64 on an aarch64 exec.

**This is a dev-environment gap, not a toolchain wiring bug.** The hermetic gcc toolchains are correctly
configured; the `linux_x86_64` build simply cannot run on an aarch64 machine without either:
- An x86_64 exec machine (CI uses x86_64 runners for x86_64 target builds), or
- A cross-compiler toolchain registered with `exec_compatible_with = [linux, aarch64]` and
  `target_compatible_with = [linux, x86_64]` (i.e. an aarch64-hosted cross to x86_64 — a different,
  additional toolchain entry, not a change to the existing one).

The `linux_arm64` target build (`--platforms=//bazel/platforms:linux_arm64`) should succeed on this machine
because `gcc_toolchain_aarch64` has `exec_compatible_with = [linux, aarch64]`. That is the appropriate
verification target for this dev environment. The x86_64 glibc floor must be verified on an x86_64 CI
runner (or an emulation/qemu exec platform, which the repo does not currently register).

**Steps:**
1. **Verify the floor** (the load-bearing action). Build and inspect, per arch.
   IMPORTANT: use an **absolute repo path** for `find`; `bazel-bin` is a relative symlink and
   will not resolve if the command is run from any directory other than the repo root.
   Replace `$REPO` with the absolute path to this repository (e.g. `/home/bits/go/src/github.com/DataDog/datadog-agent`).
   - **arm64** (runnable on this aarch64 dev machine):
     ```
     REPO=/home/bits/go/src/github.com/DataDog/datadog-agent
     cd "$REPO"
     bzl build @cpython//:python_unix_shared --platforms=//bazel/platforms:linux_arm64
     LIBPY=$(find -L "$REPO/bazel-bin" -name 'libpython3*.so*' 2>/dev/null | grep -v 'python3.12\|rules_python' | head -1)
     echo "LIBPY=$LIBPY"
     objdump -T "$LIBPY" | grep -oP 'GLIBC_\K[0-9.]+' | sort -V | tail -1   # expect <= 2.23
     ```
     Note: use `find -L` (follow symlinks) because bazel-bin is a symlink into the Bazel output base.
     Exclude `rules_python` Python 3.12 libraries (Bazel tooling runtime) with `grep -v`.
     Do NOT pass `--output_base`; let Bazel use its default persistent cache so bazel-bin resolves
     to `~/.cache/bazel/_bazel_bits/…` and survives across sessions.
   - **x86_64** (must run on an x86_64 CI runner — fails at toolchain resolution on aarch64 host):
     ```
     REPO=/path/to/datadog-agent
     cd "$REPO"
     bzl build @cpython//:python_unix_shared --platforms=//bazel/platforms:linux_x86_64
     LIBPY=$(find -L "$REPO/bazel-bin" -name 'libpython3*.so*' 2>/dev/null | grep -v 'python3.12\|rules_python' | head -1)
     echo "LIBPY=$LIBPY"
     objdump -T "$LIBPY" | grep -oP 'GLIBC_\K[0-9.]+' | sort -V | tail -1   # expect <= 2.17
     ```
     See P3 investigation findings above — the toolchain resolution error on aarch64 is expected and correct;
     the x86_64 floor check belongs in the CI pipeline on an x86_64 runner.
   - **arm64 result (verified 2026-06-09, re-confirmed 2026-06-09):** `BUILD_EXIT:0`. Max glibc symbol version in
     `libpython3.13.so.1.0` = **2.17** (floor <= 2.23 required). PASSES.
     Artifact (via default output base, absolute path required):
     `$REPO/bazel-bin/_solib_local/_U_A_A+http_Uarchive+cpython_S_S_Cpython_Uunix___Uexternal_S+http_Uarchive+cpython_Spython_Uunix_Slib/libpython3.13.so.1.0`
     Physical path: `~/.cache/bazel/_bazel_bits/067d06d7fe4ac64a0a31948756f59f5f/execroot/_main/bazel-out/aarch64-fastbuild/bin/external/+http_archive+cpython/python_unix/lib/libpython3.13.so.1.0`
2. **If it passes:** Phase 3 is done — fold the check into the Phase 5 gate (run it on `libpython` too).
3. **If it fails:** the fix is *wiring*, not rebuilding. Confirm `configure_make` resolves the GCC toolchain
   via `toolchains`/exec-platform rather than the LLVM toolchain (last-registered fallback in
   `MODULE.bazel:305`). rules_foreign_cc honors the resolved `cc_toolchain`; pin it by giving the
   `configure_make` target `target_compatible_with` the Linux platforms and ensuring GCC is selected (it
   currently borrows `@llvm_toolchain_llvm//:ar` only for the `AR` flag — verify CC/CFLAGS come from GCC). If a
   stray host include leaks, add `--sysroot` from the gcc_toolchain.
4. **Bundling** is already wired: `libpython_symlink` + `python_pkg` (`deps/cpython.BUILD.bazel:415-460`) flow
   into `//packages/install_dir:embedded` → `packages/agent/linux` deb/rpm. No new bundling work.

**Key files:** `deps/cpython.BUILD.bazel`, `deps/cpython.MODULE.bazel`, `MODULE.bazel` (toolchain registration
order).

**Risks:** rules_foreign_cc toolchain resolution is subtle; the LLVM toolchain being registered last
(`MODULE.bazel:305`, "last to avoid taking precedence") is intentional for macOS but means on Linux you must
confirm GCC actually wins for the configure_make exec. macOS `libpython` floor is the `--macos_minimum_os=12.0`
deployment target, separately gated.

**What disappears:** `nix/embedded-python.nix`; the `EMBEDDED_PYTHON` env-var honoring in `tasks/agent.py:106-112`.

---

## Phase 4 — omnibus replacement (finish packaging migration)

**Goal:** Produce deb/rpm/macOS/Windows entirely from `rules_pkg`, fed by Phase 1 `go_binary`s; retire omnibus
and Ruby.

**Current state (largely done):**
- `packages/agent/linux/BUILD.bazel` already emits `pkg_deb` + `pkg_rpm` from `pkg_tar(:whole_distro_tar)`,
  assembling `//packages/agent/dependencies`, `/product`, `//packages/install_dir:embedded` + `:etc`,
  installer symlinks, and OSS license aggregation (`package_licenses`).
- Install prefix is parameterized: `//:install_dir` (`BUILD.bazel:901`), `//:output_config_dir`,
  `//:is_standard_install`. The non-root path hacks are unnecessary.
- Installer package migration is mid-flight with a written comparison methodology
  (`packages/installer/MIGRATION_PLAN.md`: prebuilt → BUILD files → CI file-tree + metadata diff → fix-diffs →
  cutover).

**What rules_pkg does NOT yet cover (the residual):**
- Agent product binaries still come from the **hybrid** `prebuilt_file` repos (closed by Phase 1 step 6).
- omnibus finalize/permission/symlink scripts not yet fully reproduced — enumerate from
  `omnibus/config/projects/agent.rb` and `omnibus/config/software/datadog-agent*.rb`; map each to
  `pkg_mkdirs`/`dd_agent_pkg_mklink`/`pkg_attributes`/postinst scriptlets.
- Per-pipeline compression injection (`get_compression_level`, TODO ABLD-364).
- macOS `.app`/`.pkg` and Windows MSI parity (`packages/macos/app`, `packages/windows`) — verify against
  omnibus output.

**Steps:**
1. Apply the installer plan's **comparison gate** to the agent package: file-tree diff + control/spec/scriptlet
   diff between the omnibus deb/rpm and `bzl build //packages/agent/linux:debian` (`:redhat`). Drive any
   remaining differences to zero.
2. Close the finalize-script gap (permissions, symlinks, postinst) in `packages/agent/**`.
3. Cutover: switch the release pipeline's packaging step from omnibus to the `rules_pkg` targets.

**Key files:** `packages/agent/**`, `packages/macos/**`, `packages/windows/**`, `packages/install_dir/**`,
`omnibus/**` (delete at cutover).

**Risks:** scriptlet/permission parity is where omnibus parity migrations historically slip; the comparison gate
is mandatory before cutover. Per `AGENTS.md`, packaging changes don't run E2E on PR branches — gate with the
file-tree/metadata diff in CI and `qa/rc-required`.

**What disappears:** the entire `omnibus/` tree; the omnibus Ruby Gemfile and Ruby 2.7 dependency
(`nix/` Ruby 2.7.8 build work, the GCC-14 Ruby fix commit); omnibus `replace_env` CGO-scrubbing problem;
omnibus PathFetcher and the GOMODCACHE-in-repo problem; `DD_SYS_BIN_DIR`/`DD_LOG_DIR` and other non-root path
shims; `nix/` cross-toolchain derivation (replaced by registered `gcc_toolchain_*`).

---

## Phase 5 — Verification: glibc floor CI gate

**Goal:** Replace `tasks/nix-verify.sh --suite=release` with a Bazel test that enforces the glibc floor on both
the agent ELF and `libpython`, per platform.

**Current state:** `tasks/nix-verify.sh:171-234` does the check in bash via `objdump -T | grep GLIBC_` with
per-arch floors (2.17 / 2.23). No Bazel equivalent exists. Note that script itself flags that Nix's
`libpython` failed the floor — Phase 3 must pass first for this gate to be green.

**Steps:**
1. Write a custom rule (preferred over `sh_test` per `bazel/AGENTS.md` "shell as last resort"):
   `bazel/rules/glibc_floor/glibc_floor_test.bzl`. Inputs: an ELF target + a `max_version` string. The rule
   runs `objdump` from the resolved `cc_toolchain` (`use_cc_toolchain()` → `find_cpp_toolchain`), greps the
   max `GLIBC_` symbol version, compares to the floor, exits non-zero on violation. Use `ctx.actions.run` (no
   shell). If a quick stopgap is needed, a `run_binary` wrapping a small Go/py helper is acceptable; avoid
   `sh_test`.
2. Instantiate per platform/target:
   - `//cmd/agent` ELF — floor 2.17 on `linux_x86_64`, 2.23 on `linux_arm64`.
   - `@cpython//:python_unix_shared` (`libpython3.13.so.1.0`) — same floors.
   - Apply per-platform via `--platforms` + `target_compatible_with`; macOS/Windows targets skip via
     `@platforms//:incompatible`.
3. Wire into CI: `bzl test //bazel/rules/glibc_floor/... --platforms=//bazel/platforms:linux_x86_64` and the
   arm64 variant. Mind exit code 3 (tests failed) vs 1 (build failed) per `bazel/AGENTS.md`.

**Key files:** `bazel/rules/glibc_floor/` (new), `packages/agent/linux/BUILD.bazel` or a top-level
`//ci:floor_checks` test suite, CI config.

**Risks:** `objdump` must come from the hermetic toolchain, not host `PATH`, or the check is non-hermetic.
macOS floor is enforced via `--macos_minimum_os` at link time, not objdump — keep that a separate assertion.

**What disappears:** `tasks/nix-verify.sh`; `RESULTS.md`; the `--suite=release` harness.

---

## Phase 6 — Toolchain strictness sweep (gating cutover)

**Goal:** Build every target under the hermetic GCC 11.4/12.3 (and MinGW GCC 14.2) toolchains and fix new
errors before cutover.

**Current state:** The hermetic toolchains are registered (`MODULE.bazel:234-306`) but not all of `//pkg/...`,
`//comp/...`, `//cmd/...` has been compiled through them — ~36% of packages lack Gazelle-generated BUILD files,
and the agent binaries built via `dda`/host `go` have never seen GCC 14. The `noop_version` signature bug
already surfaced via Nix proves new strictness errors are latent.

**Steps:**
1. `bzl build //cmd/... //pkg/... //comp/... --platforms=//bazel/platforms:linux_x86_64` (and each platform).
2. Fix GCC-14 / strict-C breakages (the `noop_version` signature class already found via Nix). Track in a
   checklist; each fix is a normal Go/C change, not Bazel.
3. Run `bzl test //...` per platform; reconcile `system-probe` eBPF tests (see workspace section — they no
   longer need `sudo` under Bazel sandboxing).

**Key files:** any cgo/C source flagged by GCC 14 (Go files with `// #cgo` blocks, `pkg/**/*.c`,
`pkg/security/**`); BUILD files for not-yet-Gazelle'd packages.

**Risks / open questions:** the unbuilt ~36% of packages may surface a long tail of strictness fixes; budget
for it. eBPF tests requiring a privileged kernel can't run in the unprivileged sandbox — isolate them
(`manual` tag, dedicated runners) rather than blocking the default `bzl test //...`.

**What disappears:** ad-hoc Nix-side strictness patches; the `sudo`-PATH workaround in the system-probe test runner.

---

## Workspace compatibility (independent of build-system choice)

Hermetic Bazel makes `workspaces create --repo datadog-agent` work without the devcontainer `base` /
`base-compat` feature because the build:
- **never writes system paths** — outputs go to `bazel-out`/`bazel-bin`; install prefix is `//:install_dir`, a
  build parameter, so nothing touches `/usr/bin`, `/var/log/datadog`, `/opt/omnibus`.
- **needs no root** — no omnibus PathFetcher copying the tree to read-only locations; no `useradd`.
- **does not collide with the pre-baked `bits` user** — the `base` feature's `useradd` collision is moot once
  no install-to-system step runs. The MEMORY note (buildimage version skew) is latent precisely because the
  collision only fires on the omnibus path.
- **brings its own toolchains** — GCC/LLVM/MinGW/Go/Python/Rust are all Bazel-managed; the VM needs only
  `bazelisk` + a C runtime, no Conda, no Ruby, no Docker.

**Remaining workspace provisioner requirements (NOT solved by the build system):** the three hard-coded
provisioner scripts (git/credential/clone bootstrap) are orthogonal to Bazel and must stay. Document them in
the workspace template; they are not deleted by this migration.

`sudo` for `system-probe` tests: Bazel's `linux-sandbox` (falling back to `processwrapper-sandbox` in
unprivileged containers, per `bazel/AGENTS.md`) runs tests without `sudo`. Verify eBPF tests under the sandbox;
where a real privileged kernel is required, mark those targets `manual` + a tag, run them only on dedicated CI
runners — never gate the default `bzl test //...` on `sudo`.

---

## Cutover sequence & global deletions

1. Phase 6 strictness sweep green on all platforms.
2. Phase 3 floor verified green; Phase 5 gate enforcing it in CI.
3. Phase 1 binaries swapped into packaging; Phase 4 comparison gate at zero diffs.
4. Flip `enable_bazel` default → `True` (already a no-op once packaging consumes real targets); bake one release.
5. Delete: `enable_bazel` flag, `prebuilt_file` agent blocks (`MODULE.bazel`), `omnibus/`, `rtloader/**/CMakeLists.txt`,
   `nix/`, `flake.nix`, `tasks/nix-verify.sh`, `RESULTS.md`, the `EMBEDDED_PYTHON`/`DD_RTLOADER_PYTHON3_ROOT`/
   non-root-path shims, the Ruby 2.7 dependency, and the legacy `go_build` body in `tasks/agent.py`.

## Appendix: Binary × Flavor × Build-Tag Matrix

Derived from `tasks/build_tags.bzl` (canonical tag sets), `tasks/build_tags.py` (`build_tags` mapping,
`compute_build_tags_for_flavor`), `packages/agent/product/BUILD.bazel` (packaging consumers), and
`MODULE.bazel:451-487` (hybrid prebuilt_file repos). Required before writing the `gotags`/`select()` wiring.

**Notes:**
- `COMMON_TAGS` (`grpcnotrace`, `retrynotrace`, `no_dynamic_plugins`, `trivy_no_javadb`) are added to every binary by `get_default_build_tags` — they go into `gotags` unconditionally.
- `FIPS_TAGS` (`goexperiment.systemcrypto`, `requirefips`) are added on the fips flavor; wire via `//bazel/platforms:fips` constraint. Note: `requirefips` is excluded on Windows (`WINDOWS_EXCLUDED_TAGS`).
- `LINUX_ONLY_TAGS` (`netcgo`, `systemd`, `jetson`, `linux_bpf`, `nvml`, `pcap`, `podman`, `trivy`, `crio`) are filtered out on non-Linux platforms by `filter_incompatible_tags`; model as `select()` on `@platforms//os:linux`.
- The "Static bundle tag" column records the `bundle_<name>` tag that uniquely identifies the binary (to be stamped by the Gazelle extension). No such tag exists for most binaries today — this is the tag the extension must add.
- "Conditional tags" are tags that are absent in certain build modes: `nvml` is dropped when `--no-glibc` (`glibc=False` in `tasks/agent.py:143-144`); `pcap` is excluded from unit tests (`UNIT_TEST_EXCLUDED_TAGS`).
- Flavors column lists only flavors defined in `build_tags.py`'s `build_tags` mapping for that binary.
- Prebuilt repo = the `prebuilt_file(name=...)` entry in `MODULE.bazel`; "none" means not yet captured as a hybrid repo (packaging TODO).

| Binary | `cmd/` path | Flavors shipped | Prebuilt repo | Static bundle tag | Base tags (from `build_tags.bzl` + COMMON) | Heroku delta vs base | FIPS delta | Conditional tags (Linux-only or mode-gated) |
|---|---|---|---|---|---|---|---|---|
| **agent** | `cmd/agent` | base, iot, heroku, fips | `@agent_binary` | `bundle_agent` (proposed) | `consul containerd cri crio datadog.no_waf docker ec2 etcd fargateprocess jetson jmx kubeapiserver kubelet ncm netcgo nvml oracle orchestrator otlp podman python sharedlibrarycheck systemd systemprobechecks trivy zk zlib zstd cel` + COMMON | remove: `containerd cri crio docker ec2 fargateprocess jetson kubeapiserver kubelet nvml oracle orchestrator podman systemd trivy cel`; add: `bundle_installer` (existing in `ALL_TAGS`) | add: `goexperiment.systemcrypto requirefips` | Linux-only: `netcgo systemd jetson nvml podman trivy crio`; `nvml` also dropped with `--no-glibc` |
| **iot-agent** | `cmd/iot-agent` | iot only | none | `bundle_iot_agent` (proposed) | `jetson systemd zlib zstd` + COMMON | N/A (iot-only) | none (not in fips map) | Linux-only: `systemd jetson` |
| **process-agent** | `cmd/process-agent` | base, heroku, fips | `@process_agent_binary` | `bundle_process_agent` (proposed) | `containerd cri crio datadog.no_waf docker ec2 fargateprocess kubelet netcgo podman zlib zstd` + COMMON | remove: `containerd cri crio docker ec2 kubelet podman`; keep: `datadog.no_waf fargateprocess netcgo zlib zstd` | add: `goexperiment.systemcrypto requirefips` | Linux-only: `netcgo podman crio` |
| **trace-agent** | `cmd/trace-agent` | base, heroku, fips | `@trace_agent_binary` | `bundle_trace_agent` (proposed) | `containerd datadog.no_waf docker kubelet netcgo otlp podman` + COMMON | remove: `containerd docker kubelet podman`; keep: `datadog.no_waf netcgo otlp` | add: `goexperiment.systemcrypto requirefips` | Linux-only: `netcgo podman` |
| **system-probe** | `cmd/system-probe` | base, fips | none | `bundle_system_probe` (proposed) | `datadog.no_waf ec2 linux_bpf netcgo npm nvml pcap seclmax zlib zstd` + COMMON | N/A (no heroku flavor) | add: `goexperiment.systemcrypto requirefips` | Linux-only: `linux_bpf netcgo nvml pcap`; `pcap` excluded in unit tests (`UNIT_TEST_EXCLUDED_TAGS`) |
| **cluster-agent** | `cmd/cluster-agent` | base (no heroku/iot/fips entry in map) | none | `bundle_cluster_agent` (proposed) | `clusterchecks datadog.no_waf ec2 kubeapiserver orchestrator zlib zstd cel` + COMMON | N/A | none (not in fips map) | none |
| **cluster-agent-cloudfoundry** | `cmd/cluster-agent-cloudfoundry` | base only | none | `bundle_cluster_agent_cloudfoundry` (proposed) | `clusterchecks cel` + COMMON | N/A | none (not in fips map) | none |
| **installer** | `cmd/installer` | base, fips | `@installer_binary` | `bundle_installer` (exists in `ALL_TAGS`) | `ec2` + COMMON | N/A (no heroku flavor) | add: `goexperiment.systemcrypto requirefips` | none |
| **dogstatsd** | `cmd/dogstatsd` | base, fips (dogstatsd flavor) | none | `bundle_dogstatsd` (proposed) | `containerd docker kubelet podman zlib zstd` + COMMON | N/A (no heroku flavor) | add: `goexperiment.systemcrypto requirefips` | Linux-only: `podman` |
| **otel-agent** | `cmd/otel-agent` | base, fips | none | `bundle_otel_agent` (proposed) | `kubelet otlp zlib zstd` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | none |
| **security-agent** | `cmd/security-agent` | base, fips | none | `bundle_security_agent` (proposed) | `datadog.no_waf docker ec2 netcgo zlib zstd` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | Linux-only: `netcgo` |
| **cws-instrumentation** | `cmd/cws-instrumentation` | base, fips | none | `bundle_cws_instrumentation` (proposed) | `netgo osusergo` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | none (netgo/osusergo are not in LINUX_ONLY_TAGS; ships Linux-only by product decision but tags are cross-platform) |
| **privateactionrunner** | `cmd/privateactionrunner` | base, fips | `@privateactionrunner_binary` | `bundle_privateactionrunner` (proposed) | `zlib zstd` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | none |
| **loader** (trace-loader) | `cmd/loader` | base only | `@trace_loader_binary` | `bundle_loader` (proposed) | _(empty — `LOADER_TAGS = set()`)_ + COMMON | N/A | none (not in fips map) | none |
| **secret-generic-connector** | `cmd/secret-generic-connector` | base, fips | none | `bundle_secret_generic_connector` (proposed) | _(empty — `SECRET_GENERIC_CONNECTOR_TAGS = set()`)_ + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | none |
| **serverless-init** | `cmd/serverless-init` | base, fips | none | `bundle_serverless_init` (proposed) | `otlp serverless` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | none |
| **sbomgen** | `cmd/sbomgen` | base, fips | none | `bundle_sbomgen` (proposed) | `containerd crio docker trivy` + COMMON | N/A | add: `goexperiment.systemcrypto requirefips` | Linux-only: `crio trivy` |
| **host-profiler** | `cmd/host-profiler` | base only | none | `bundle_host_profiler` (proposed) | `docker kubelet remove_all_sd` + COMMON | N/A | none (not in fips map) | Linux-only: none (`docker` excluded on Darwin by `DARWIN_EXCLUDED_TAGS` but not in LINUX_ONLY_TAGS) |
| **systray** | `cmd/systray` | base, Windows-only | none | `bundle_systray` (proposed) | Windows-only binary; no build-tag entry in `build_tags.py` — tags TBD from `cmd/systray` source | N/A | N/A | Windows-only |

### Bazel wiring summary

| Axis | Mechanism |
|---|---|
| Per-binary static tags (base flavor, cross-platform) | `gotags = [...]` literal on `go_binary` (or stamped by Gazelle extension) |
| `bundle_<name>` tag | Gazelle extension stamps onto `go_binary` under `cmd/` (extends `bazel/rules/go_build_tags`) |
| FIPS flavor | `select()` on `//bazel/platforms:fips` constraint; adds `goexperiment.systemcrypto` + `requirefips` (minus `requirefips` on Windows via platform constraint) |
| Heroku flavor | `//bazel/flags:flavor` string_flag with `config_setting`s; `select()` removes/adds tags and adjusts `deps` |
| IoT flavor | Same `//bazel/flags:flavor` flag; iot-agent is a distinct `go_binary` target at `cmd/iot-agent` |
| Linux-only tags | `select()` on `@platforms//os:linux`; non-Linux targets get the tag removed |
| `nvml` with `--no-glibc` | Named `.bazelrc` config `build:no_glibc` that appends `--@rules_go//go/config:tags=` removing `nvml` |
| `pcap` (cgo, libpcap) | Included in base `SYSTEM_PROBE_TAGS`; excluded from unit-test configs via `UNIT_TEST_EXCLUDED_TAGS` |
| `ebpf_bindata`, cross-cutting | Named `.bazelrc` configs (`build:ebpf_bindata`) composing with per-target `gotags` |
