# Bazel deb parity — final gaps plan

Closes the 3 small remaining gaps and the embedded Python long pole.
Follows on from `bazel-deb-parity.md` (which closed the bulk of the gap).

---

## Current parity status (June 2026)

**Reference deb:** `datadog-agent_7.77.1-1_arm64.deb` (149 MB, 22,564 files) from `apt.datadoghq.com`
**Bazel deb:** `datadog-agent_7.81.0-localbuild-1_arm64.deb` (~230 MB, 35,418 files)

**Raw diff:** 2,447 paths only-in-reference. After filtering accepted deviations
(`version-manifest.*`, paths containing `7.77.1`) and normalizing Python package version
strings in dist-info paths, **genuine structural gaps = ~150 non-Python files**.

The ~2,294 remaining raw gaps are all Python package version-skew: the Bazel deb installs
newer versions of third-party packages than the 7.77.1 reference (e.g. `botocore-1.42.72`
vs reference's `botocore-1.40.21`). All packages are present; only the dist-info directory
name (which embeds the version) differs. The correct parity check for site-packages is a
package-name-normalized comparison, which produces 0 genuinely missing packages.

**Remaining ~150 genuine gaps by category:**

| Category | Count | Status |
|---|---|---|
| Python `embedded/bin` entry-point scripts | ~40 | TODO — `whl_extract.bzl` doesn't generate entry-points yet |
| kerberos/GSSAPI headers (`include/krb5/`, `include/gssrpc/`, etc.) | ~43 | TODO — check krb5 dep filegroup |
| `.so` full-version symlinks (`libssl.so.3.5.5` etc.) | 7 | **ACCEPTED DEVIATION** — see note below |
| `LICENSES/` nested format + specific license files | ~32 | TODO — layout/content gaps |
| `bin/agent/dist/` stubs (`config.py`, `security-agent.yaml`, etc.) | 4 | TODO — check if in `cmd/agent/dist/` |
| `clang-bpf`, `llc-bpf` | 2 | **IMPLEMENTED** — see Gap 5 below |
| `agent-data-plane` | 1 | **REPO RULE WRITTEN** — see Gap 6 below; CI auth needed for prod fetch |
| SELinux policy (`.pp` file) | 2 | TODO — needs `genrule` to compile |
| `embedded/bin/__pycache__/` | 1 | Acceptable deviation |
| Omnibus tracking files (`.install_root`, `.installed_by_pkg.txt`, etc.) | ~4 | Acceptable deviation — omnibus bookkeeping |
| `python-scripts/` (`pre.py`, `post.py`, `packages.py`) | 4 | Acceptable deviation — omnibus install helpers, not needed at runtime |
| eBPF test objects (`btf_test.o`, `uprobe_attacher-test.o`) | 3 | Acceptable deviation — test artifacts |

**Note on `.so` full-version symlinks (7 items — accepted deviations):**

All 7 items previously listed as "TODO — add symlinks to dep rules" are **not structural
gaps** in the Bazel build. The `so_symlink.bzl` macro already generates the correct
3-tier symlink chain (real_name/soname/linker_name) for every library. The apparent gap
was an artefact of the investigation's `awk '{print $NF}'` extraction method, which for
symlink lines in `dpkg-deb -c` output returns the symlink *target* rather than the
symlink *path*, producing duplicate/incorrect entries in the comparison.

The 7 items broken down:
1. `libssl.so.3.5.5` — version skew (Bazel ships 3.5.7, ref is 3.5.5)
2. `libcrypto.so.3.5.5` — version skew (same openssl version bump)
3. `libsqlite3.so.3.50.4` — version skew (Bazel: 3.53.0, ref: 3.50.4)
4. `libxml2.so.16.0.5` — version skew (Bazel: 16.1.3, ref: 16.0.5)
5. `libxslt.so.1.1.43` — version skew (Bazel: 1.1.45, ref: 1.1.43)
6. `libexslt.so.0.8.24` — version skew (Bazel: 0.8.25, ref: 0.8.24)
7. `libdatadog-agent-rtloader.so.2` — intentional SOVERSION drop (PR #46720, 2026-02-20);
   Bazel correctly emits `.so.0` per `VERSION=0.1.0` in `rtloader/BUILD.bazel`.

**Fix implemented in comparison-gate.yml (June 2026):**

The comparison gate was fundamentally redesigned to use a **path-set comparison** instead
of a raw `diff` on full `tar tv` lines. This eliminates false positives from format
differences between omnibus and Bazel:

- **Leading `./` path prefix**: omnibus tar entries start with `./`; rules_pkg does not.
- **Ownership**: omnibus uses `root/root`; rules_pkg uses `0/0`.
- **Symlink permissions**: omnibus emits `lrwxrwxrwx`; rules_pkg emits `lrw-r--r--`.
- **Timestamps**: real timestamps vs reproducible-build epoch (2000-01-01 00:00).
- **File sizes**: same path but different binary version has different size.

The new approach:
1. `_path_set()` awk function extracts `TYPE PATH [-> SYMLINK_TARGET]` (stripping `./`)
2. `.so` version normalization still applied (`lib*.so.X.Y.Z → lib*.so.X`)
3. `diff` runs on the resulting path-set (one entry per file, not one line per char diff)
4. PASS/FAIL decision uses `pkg-path-diff.txt`; raw `pkg-diff.txt` kept for human review

Result: `embedded/lib` reduced from 345 reported false positives to **4 genuine entries**:
- `libdatadog-agent-rtloader.so -> .so.2` and `.so.2 -> .so.0` — ACCEPTED (SOVERSION change)
- `libkadm5clnt_mit.so.12 -> libkadm5clnt.so` — different symlink target (krb5 versioning)
- `libkadm5srv_mit.so.12 -> libkadm5srv.so` — different symlink target (krb5 versioning)

`ACCEPTED_DIFF_PATTERNS` updated to match path-set format (no `./` prefix).

Do NOT add `pkg_symlink`/`dd_agent_pkg_mklink` for `.so` chains — `so_symlink.bzl` already
emits the correct chain, and adding more would cause duplicate-path build errors.

**Constraint:** `bzl build //packages/agent/linux:debian` must produce a complete deb
without any omnibus pipeline having run first. The Bazel build path is fully independent
of omnibus. Option A (importing omnibus-produced artifacts) is explicitly ruled out.

---

## Ground-truth corrections (found by reading the code)

1. **Gap 1 is a binary, not a symlink.** `datadog-agent.rb:140-142` installs the
   installer binary at `embedded/bin/installer`. The `finalize.rb:90-93` symlink is
   a fallback only for heroku builds. `//cmd/installer:installer` is a ready-to-package
   `go_binary`. Do NOT add a `dd_agent_pkg_mklink` — a symlink where omnibus ships a
   regular file would be flagged by `tar tv`.

2. **`checks.d` is created by `finalize.rb`, NOT the postinst.** The postinst delegates
   to `embedded/bin/installer postinst` which creates `/etc/datadog-agent`,
   `/var/log/datadog`, `processes.d` etc — but not `checks.d`. It must ship in the
   deb payload via `pkg_mkdirs`.

3. **`architecture` in `pkg_deb` does not support `select()`.** The toolchain reports
   `cpu="local"` which is unreliable. Use `architecture_file` fed by a generated file
   using the same `_extract_arch(ctx, ..., "deb")` that already produces the correct
   `{arch_deb}` in the deb filename — guaranteeing the control field matches.

4. **`@cpython//:runtime_deps` is the right building block for 4a**, not
   `//rtloader:python_env_transitive`. `runtime_deps` walks only `python_pkg` and its
   native deps, avoiding the rtloader/`three.so` duplication.

5. **The `datadog-*` integration wheels are NOT on public PyPI.** `datadog-postgres`,
   `datadog-snmp` etc. on PyPI are name-reservation placeholders (758 bytes, v0.0.1).
   The real wheels are published only to Datadog's TUF-secured distribution system
   (`dd-integrations-core-wheels-build-stable.datadoghq.com`), used by the runtime
   `datadog-agent integration install` command — not a pip index. This means `pip.parse`
   from PyPI cannot fetch them; they must be built from `integrations-core` git source.

6. **The integrations S3 wheel cache is also ruled out.** It is populated inside omnibus's
   `build do` block. Pulling from it is an omnibus dependency in disguise, even though the
   bucket is public-read.

7. **Option A (import omnibus-produced artifact) is ruled out** per the constraint.
   **Option C (fleet-installer runtime fetch) is ruled out** — does not produce a complete deb.
   **Only Option B1 (build entirely within Bazel from public sources) satisfies the constraint.**

---

## Gap 1 — installer binary in `embedded/bin/installer`

**File:** `packages/agent/product/BUILD.bazel`

Add a `pkg_files` target mirroring `system_probe_binary`:
```python
pkg_files(
    name = "installer_binary",
    srcs = ["//cmd/installer:installer"],
    attributes = pkg_attributes(mode = "755"),
    prefix = "embedded/bin",
)
```

Add `:installer_binary` to `all_files`, gated to non-heroku linux flavors
(same `select` as `//pkg/discovery/module/rust:all_files` at `product/BUILD.bazel:58-69`).
Remove the `# TODO: installer` comment.

**Verification:** `dpkg-deb -c <deb>` shows `./opt/datadog-agent/embedded/bin/installer`
as a regular file mode 0755 (not a symlink). No `/usr/bin/datadog-installer` entry in
the payload (created by postinst at install time, not packaged).

**Effort: S (~30 min)**

---

## Gap 2 — `/etc/datadog-agent/checks.d/` empty directory

**File:** `packages/agent/product/BUILD.bazel`

Extend the existing `dirs` `pkg_mkdirs` target to include `checks.d` alongside
the other `etc/datadog-agent/` subdirs:
```python
pkg_mkdirs(
    name = "dirs",
    dirs = [
        ...existing entries...,
        "checks.d",   # ← add
    ],
    prefix = select(ETC_DIR_SELECTOR),
)
```

**Verification:** `dpkg-deb -c <deb>` shows `./etc/datadog-agent/checks.d/` as an
owned empty directory.

**Effort: XS (~10 min)**

---

## Gap 3 — `Architecture: all` → `amd64`/`arm64`

**Files:**
- `packages/rules/package_naming.bzl` — add `deb_architecture_file` rule
- `packages/agent/linux/BUILD.bazel` — use `architecture_file` on `pkg_deb`

### Step 1: new rule in `package_naming.bzl`

```python
def _deb_architecture_file_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    arch = _extract_arch(ctx, cc_toolchain.cpu, "deb")
    out = ctx.actions.declare_file(ctx.label.name + ".txt")
    ctx.actions.write(out, arch)
    return DefaultInfo(files = depset([out]))

deb_architecture_file = rule(
    implementation = _deb_architecture_file_impl,
    attrs = { "_cc_toolchain": attr.label(default = "@bazel_tools//tools/cpp:current_cc_toolchain") },
    toolchains = ["@bazel_tools//tools/cpp:toolchain_type"],
    fragments = ["cpp"],
)
```

### Step 2: use it in `packages/agent/linux/BUILD.bazel`

```python
deb_architecture_file(name = "arch_deb_file")

pkg_deb(
    name = "debian",
    ...
    architecture_file = ":arch_deb_file",
    # Remove the TODO comment and the # architecture = "$(COMPILATION_MODE)" stub
    ...
)
```

Do not set both `architecture` and `architecture_file` — `deb.bzl:51-55` rejects that.

**Verification:** `dpkg-deb -f <out>.deb Architecture` returns `arm64` (aarch64 host)
or `amd64` (x86_64 CI), matching the `{arch_deb}` in the deb filename.

**Effort: S–M (~1–2 hours)**

---

## Gap 4a — embedded Python interpreter + stdlib

**File:** `packages/install_dir/embedded/BUILD.bazel`

Currently ships only `etc/README.md`. Wire `@cpython//:runtime_deps` here so the
interpreter flows through `//packages/install_dir:embedded` → `agent_components`.

`@cpython//:runtime_deps` (`deps/cpython.BUILD.bazel:333-344`) assembles:
`embedded/bin/python3.13`, `embedded/bin/python3` symlink, `embedded/lib/python3.13/`
(stdlib, minus `test/` and `*.exe`), `embedded/lib/libpython3.13.so.1.0` + stable-ABI
symlinks, and native C deps (openssl, sqlite, zlib, xz, libffi, bzip2, mpdecimal).

```python
pkg_filegroup(
    name = "all_files",
    srcs = [
        ":etc_readme",
        "@cpython//:runtime_deps",     # ← add
    ],
)
```

**Duplicate-path handling:** `runtime_deps` may overlap with entries already in
`packages/agent/dependencies:all_files` (native C deps). Let the comparison gate's
`tar tv` surface exact collisions; dedupe by removing overlapping entries from one side.

**Verification:** `dpkg-deb -c <deb>` shows `./opt/datadog-agent/embedded/bin/python3.13`,
`./opt/datadog-agent/embedded/bin/python3 -> python3.13` (symlink),
`./opt/datadog-agent/embedded/lib/python3.13/` (stdlib tree),
`./opt/datadog-agent/embedded/lib/libpython3.13.so.1.0`.
No duplicate-path build errors.

**4a is fully independent of 4b.** After 4a lands: functioning agent with interpreter
present, but Python-based integration checks won't load (no site-packages, no conf.d
per-check). Not at omnibus parity but useful for incremental gate progress.

**Effort: M (~half day, mostly resolving duplicate-path collisions)**

---

## Gap 4b — integrations-py3 site-packages — IMPLEMENTED

**Status: structurally complete as of June 2026.**

`deps/integrations/BUILD.bazel` now implements the full wheel assembly:
- Set (a): ~82 third-party wheels from internal CDN via `http_file` + `genrule` rename rules
  (aerospike, botocore, cryptography, kubernetes, psutil, pymongo, pysnmp, etc.)
- Set (b): ~200 per-check datadog_* wheels built from `@integrations_core` source via genrule
  (active_directory, activemq, … zk, zscaler_private_access)
- `multi_whl_extract` assembles all wheels into a single `site_packages_tree` TreeArtifact
- `site_packages_files` routes the tree to `embedded/lib/python3.13/site-packages`
- `packages/install_dir/embedded/BUILD.bazel` wires `deps/integrations:site_packages_files`
  into the embedded tree via `all_files`

**Verification (June 2026):** fresh build has 30,222 site-packages entries vs 19,510 in
reference omnibus deb (7.77.1). Package-name normalized comparison shows only 2 "missing"
items — both are version-layout changes, not missing packages:
- `decorator.py` — decorator 5.2.1 used single-file layout; Bazel ships 5.3.1 as `decorator/` pkg
- `tests/` — cryptography 46.0.5 (ref) shipped tests in wheel; 46.0.7 (Bazel) removed them

All 2293 file-level "only-in-ref" paths are version-skew artifacts (older dist-info directories,
files removed in newer package releases). The Bazel deb has ALL structurally required packages.

**Comparison methodology note:** The raw `comm -23` comparison will always show ~2293 site-packages
paths as "only-in-ref" because dist-info directory names embed version strings. The correct exit
condition for this category is the package-name-normalized comparison (strip version from dist-info
names, normalize case/dashes, comm -23) which produces 0 genuinely missing packages.

### Original design (for reference)

### The single-pin strategy

**Pin `integrations-core` at a commit.** One version pin yields:
- `.deps/resolved/*.txt` — lockfile for set (a) third-party wheels (PyPI)
- Per-check source trees — wheel source for set (b) datadog wheels
- `<check>/datadog_checks/<check>/data/{auto_conf.yaml,...}` — conf.d example configs

This is the clean single-source-of-truth answer: one pin, public sources only, no omnibus.
The pinned commit mirrors what `INTEGRATIONS_CORE_VERSION` in `tasks/omnibus.py:126` tracks.

### Step 0 (prerequisite) — 3.13 toolchain retarget

`MODULE.bazel:315` pins the `rules_python` toolchain default to **3.12**, but the
embedded interpreter (`@cpython`) and the resolved lockfiles are **3.13**. Add a 3.13
`python.toolchain(...)` alongside the existing 3.12 one. The new `pip.parse` for set (a)
MUST declare `python_version = "3.13"` to resolve `cp313`/`manylinux` wheels that
load against the 4a interpreter.

### Step 1 — Pin integrations-core (`MODULE.bazel`)

Add a sha256-pinned `http_archive` or `git_override`-style repo for `integrations-core`
at a chosen commit, exposing the lockfiles and per-check source trees as Bazel-visible files.

### Step 2 — `pip.parse` hub for third-party deps (set a)

```python
pip.parse(
    hub_name = "py_integrations_deps",
    python_version = "3.13",
    requirements_lock = "@integrations_core//:.deps/resolved/linux-x86_64_3.13.txt",
    # per-arch: either two hubs or rules_python multi-platform support
)
use_repo(pip, "py_integrations_deps")
```

Mirrors the existing `py_dev_requirements` parse. Per-arch lockfiles require either two
hubs selected by platform or `rules_python`'s multi-platform support.

### Step 3 — Build the `datadog-*` check wheels (set b)

New Bazel rules under `deps/integrations/` (new directory):
- A rule/macro for `datadog_checks_base`, `datadog_checks_downloader`, and each enabled
  check that runs the hatchling build (`py_wheel` from rules_python, or a `run_binary`
  wrapping `pip wheel . --no-deps --no-index`) against the pinned integrations-core source.
- The enabled-check set replicates omnibus's `excluded_folders` logic (recipe lines 33-50)
  and the `dda inv agent.collect-integrations` selection (recipe line 132). This must be
  a curated Bazel `.bzl` constant, not the full integrations-core set.
- **First implementation task: read `.deps/resolved/<platform>_3.13.txt` from the pinned
  commit** to confirm no entries are Datadog forks absent from public PyPI and no
  sdist-only entries that would require in-build compilation.

### Step 4 — Assemble site-packages

A rule that installs sets (a) and (b) into `embedded/lib/python3.13/site-packages/`
as a tree artifact using the 4a interpreter, then packages it with `pkg_files`/
`pkg_filegroup` (prefix `embedded/lib/python3.13/site-packages`).

### Step 5 — Wire into agent_components

Wire the new site-packages and conf.d `pkg_filegroup` targets into
`packages/install_dir/embedded/BUILD.bazel`'s `all_files` (same place as 4a).

### conf.d relocation (from integrations-core source, not wheel metadata)

Omnibus copies `<check>/datadog_checks/<check>/data/{conf.yaml.example, auto_conf.yaml,
metrics.yaml}` → `etc/datadog-agent/conf.d/<check>.d/`, **then deletes those same files
from site-packages** (recipe line 213). With the integrations-core source pin available,
the example configs come directly from the source tree — no wheel introspection needed.

A `pkg_files` rule globs the `data/*.yaml` files per check from the pinned source and
places them at `etc/datadog-agent/conf.d/<check>.d/`. The same `data/` entries must be
**excluded** from the site-packages tree to match the omnibus deletion. SNMP profile
folders (`profiles/`, `default_profiles/`) follow the same pattern.

### Files that change

- `MODULE.bazel` — integrations-core pin; 3.13 toolchain; `pip.parse` hub for set (a)
- `deps/integrations/BUILD.bazel` + `.bzl` (new) — wheel-build rules, enabled-check list,
  site-packages assembly, conf.d relocation
- `packages/install_dir/embedded/BUILD.bazel` — add site-packages + conf.d filegroups

### Effort and risk

| Sub-step | Effort | What makes it hard |
|---|---|---|
| Step 0 (3.13 retarget) | S | Mechanical toolchain registration |
| Step 1 (integrations-core pin) | S | Standard `http_archive` pattern |
| Step 2 (set-a pip.parse) | M | Per-arch lockfiles; confirming all entries are on PyPI |
| Step 3 (set-b datadog wheels) | M | Pure-Python build; volume (~100 checks) |
| Step 4+5 (assembly + conf.d) | M | Correct exclusion of data/ from site-packages |
| Native extension handling | M–L | aerospike, psycopg, pymqi, cryptography — `cp313` manylinux wheels; any sdist-only entries need in-build compilation |
| Co-resolution consistency | L | Reproducing `final_constraints-py3.txt` + `pip check` in Bazel |
| FIPS patchelf (cryptography, psycopg) | M | Per-flavor `.so` rpath rewrite actions |

**Net: 4b total is XL (multi-week), dominated by native extensions + co-resolution + FIPS.**

---

## Sequencing

```
Gap 2 (checks.d, XS) ──┐
Gap 1 (installer, S)    ├── parallelizable, same PR
Gap 3 (arch, S-M)  ─────┘

Gap 4a (Python interpreter, M) ── independent; deb is useful but not parity after this

Gap 4b (site-packages) ── DONE: all wheels assembled in deps/integrations/BUILD.bazel

Omnibus retirement gate: stays allow_failure:true until all non-site-packages gaps closed
```

---

## Key files

| File | Gap |
|------|-----|
| `packages/agent/product/BUILD.bazel` | 1 (installer `pkg_files`), 2 (`checks.d` in `dirs`) |
| `packages/agent/linux/BUILD.bazel` | 3 (`architecture_file` on `pkg_deb`) |
| `packages/rules/package_naming.bzl` | 3 (new `deb_architecture_file` rule) |
| `packages/install_dir/embedded/BUILD.bazel` | 4a (`@cpython//:runtime_deps`), 4b (site-packages) |
| `MODULE.bazel` | 4b (integrations-core pin, 3.13 toolchain, `pip.parse` hub) |
| `deps/integrations/BUILD.bazel` + `.bzl` (new) | 4b (wheel-build rules, conf.d relocation) |
| `deps/cpython.BUILD.bazel` | 4a/4b reference: `runtime_deps`, `python_pkg` |
| `omnibus/config/software/datadog-agent-integrations-py3.rb` | 4b reference: enabled-check set, excluded_folders |
| `packages/agent/product/BUILD.bazel` | 5 (`llvm_bpf_binaries` pkg_files), 6 (`agent_data_plane_binary` pkg_files) |
| `bazel/toolchains/agent_data_plane/agent_data_plane_configure.bzl` | 6 (new repo rule) |
| `MODULE.bazel` | 6 (`agent_data_plane_configure` extension + hashes) |

---

## Gap 5 — clang-bpf / llc-bpf — IMPLEMENTED

**Status: implemented June 2026.**

Added `llvm_bpf_binaries` pkg_files to `packages/agent/product/BUILD.bazel`:
```python
pkg_files(
    name = "llvm_bpf_binaries",
    srcs = ["@llvm_bpf//:bin/clang-bpf", "@llvm_bpf//:bin/llc-bpf"],
    attributes = pkg_attributes(mode = "755"),
    prefix = "embedded/bin",
    target_compatible_with = ["@platforms//os:linux"],
)
```
Wired into `all_files` base srcs (ships to all Linux flavors).
Source: the existing `@llvm_bpf` repo (LLVM 12.0.1 BPF binaries, public S3, pinned in
`MODULE.bazel:188-204`). `llvm-strip` intentionally excluded.

**Cross-arch caveat:** `@llvm_bpf` selects by build-host arch. Correct for native
per-arch builds; if cross-compilation is added later, the repo rule needs reworking to
use target arch (like `@agent_data_plane` below does).

---

## Gap 6 — agent-data-plane — BINARY + LICENSE FILES WIRED (June 2026)

**Status: binary + license files wired; production fetch needs Vault role.**

`agent-data-plane` (the Saluki data plane) is a pre-built binary from
`https://binaries.ddbuild.io/saluki/` — not built from source in this repo.

**What was implemented (June 2026 — iteration 5):**
- `bazel/toolchains/agent_data_plane/agent_data_plane_configure.bzl`:
  - **Fixed binary extraction path**: the download branch now uses
    `mv opt/datadog-agent/embedded/bin/agent-data-plane bin/agent-data-plane-ARCH`
    (was incorrectly `mv agent-data-plane bin/...` assuming root-level placement).
  - **Added license extraction**: `cp -r opt/datadog/agent-data-plane/LICENSES licenses/LICENSES-ARCH`
    and `cp opt/datadog/agent-data-plane/LICENSE-3rdparty.csv licenses/LICENSE-3rdparty-ARCH.csv`.
  - **Local-override branch extended**: creates empty stub `licenses/` files so that
    filegroup targets resolve at analysis time even without a real tarball.
  - **Generated BUILD.bazel extended**: adds `license_csv` alias, `_lic_csv_*` filegroups,
    `license_csv_files` pkg_files (with arch-selecting `renames`),
    `license_dir_files_amd64` and `license_dir_files_arm64` pkg_files (with per-arch
    `strip_prefix`, since glob() cannot appear in select()).
- `packages/agent/product/BUILD.bazel`:
  - Added `agent_data_plane_license_csv_files` (`pkg_filegroup` wrapping
    `@agent_data_plane//:license_csv_files`) — ships CSV to `LICENSES/`.
  - Added `agent_data_plane_license_dir_files` (`pkg_filegroup` with `select()` on cpu
    picking `license_dir_files_amd64` or `license_dir_files_arm64`) — ships texts to
    `LICENSES/LICENSES/`.
  - Both wired into `all_files` under `linux_default` and `linux_fips` selects.

**Tarball layout (ground truth: omnibus/config/software/datadog-agent-data-plane.rb lines 56-58):**
```
opt/datadog-agent/embedded/bin/agent-data-plane   → embedded/bin/agent-data-plane
opt/datadog/agent-data-plane/LICENSES/            → LICENSES/LICENSES/ (nested — LICENSES dir
                                                    already exists in install_dir)
opt/datadog/agent-data-plane/LICENSE-3rdparty.csv → LICENSES/LICENSE-agent-data-plane-3rdparty.csv
```

**CAVEAT:** Tarball paths above are derived from omnibus .rb lines 56-58 and have NOT been
verified against the actual 1.1.2 tarball (binaries.ddbuild.io requires Vault auth). If the
tarball layout differs, the `mv`/`cp` paths in `agent_data_plane_configure.bzl` need adjustment.
The binary-at-root assumption of the original rule came from the local binary override path
masking the download-path bug; the .rb is now treated as ground truth for both binary and licenses.

**Local development override:**
```bash
bzl build //packages/agent/linux:debian \
  --repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=/path/to/agent-data-plane-arm64
```

**Verification (--nobuild graph check):**
`bzl build --nobuild //packages/agent/linux:debian --platforms=//bazel/platforms:linux_arm64
  --repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=...` → EXIT:0, "Build completed successfully".

**To unblock production CI fetch:**
The repo rule calls `binaries.ddbuild.io` which requires a Vault OIDC role
(`binaries.ddbuild.io`) not provisioned for this workspace. Two options:
1. Request the role from the platform/infra team for the Bazel CI runner Vault identity.
2. Add `ddtool auth token binaries.ddbuild.io --datacenter us1.ddbuild.io` as a pre-step
   in the packaging GitLab job and pass the token as a `--repo_env=DDBUILD_TOKEN=...` arg.
