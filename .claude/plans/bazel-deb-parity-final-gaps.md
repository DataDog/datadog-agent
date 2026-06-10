# Bazel deb parity — final gaps plan

Closes the 3 small remaining gaps and the embedded Python long pole.
Follows on from `bazel-deb-parity.md` (which closed the bulk of the gap).

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

## Gap 4b — integrations-py3 site-packages (Bazel-independent design)

### The wheel set: two categories with two sources

**(a) Third-party dependencies** — standard hash-pinned PyPI wheels (numpy, cryptography,
psycopg, pyyaml, aerospike, pymqi, etc.). The lockfile is
`.deps/resolved/<platform>_<pyver>.txt` in the `integrations-core` repo — exactly the
format `pip.parse()` already consumes in this repo (see `deps/py_dev_requirements_lock.txt`,
`MODULE.bazel:330-335`).

**(b) `datadog-*` integration wheels** — pure-Python wheels built from `integrations-core`
git source with `pip wheel . --no-deps --no-index` (hatchling backend) for
`datadog_checks_base`, `datadog_checks_downloader`, and each enabled check. NOT on
public PyPI (see ground-truth correction #5 above).

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

Gap 4b-step-0 (3.13 retarget, S) ── prerequisite for 4b
Gap 4b-step-1 (integrations-core pin, S)
Gap 4b-step-2 (set-a pip.parse, M)
Gap 4b-step-3 (set-b datadog wheels + conf.d, M → L for native exts + FIPS)

Omnibus retirement gate: stays allow_failure:true until 4b-step-3 is green
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
