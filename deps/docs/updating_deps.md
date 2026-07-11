# Updating a third-party C/C++ library under `deps/`

End-to-end procedure for bumping a single C/C++ dependency under `deps/`. A bump reconciles the source listing, copts/defines, the checked-in config header, and patches against the new upstream tarball — not just the sha256.

**Inputs:** `<dep-name>` (required) and `<target-version>` (optional — ask, or resolve from upstream's latest release).

Scope: C/C++ deps under `deps/` with a Bazel overlay. **Out of scope:** openssl, python, krb5 (no overlay), Go deps, Python packages.

---

## Operating principles

- **Never produce a pin-checksum from an unverified download.** Pin sha256 from a trusted upstream source (`.sha256`/`SHA256SUMS`, signed manifest, GPG-verified tarball) or ask the user.
- **Probe `HAVE_*` detections with the project's Bazel toolchain.** Linux uses a ctng glibc baseline; upstream defaults are wrong for glibc-gated functions.
- **Surface, don't silently flip.** Every non-version `HAVE_*`/`USE_*` decision goes through the user before any edit.
- **Stop on real failures.** Hash mismatch, probe failure, patch conflict — surface and pause.

---

## Supporting documents

| When you need to...                          | Read this                                  |
|-----------------------------------------------|--------------------------------------------|
| Bump the MODULE.bazel pin (manual mode only)  | [bump-version.md](bump-version.md)         |
| Refresh the checked-in config header          | [config-h-refresh.md](config-h-refresh.md) |
| Refresh patches against the new source tree   | [patches.md](patches.md)                   |
| Run the build gates and route failures        | [verification.md](verification.md)         |

**Do not load all docs at once.** Read each only when you reach the step that references it.

---

## Step 1 — Locate the dep and detect mode

Resolve everything from the dep's `http_archive` block; layout varies but the MODULE.bazel block is canonical.

- **Find the http_archive block.** Look in `deps/repos.MODULE.bazel`, then `deps/<dep-name>/<dep-name>.MODULE.bazel`. For deps using `module_utils.bzl` (via `use_repo_rule`), the state is in `release.json` under `<NAME>_VERSION` / `<NAME>_SHA256`. If none found, stop — special case (openssl, python, krb5) or no overlay yet.
- **Capture from the http_archive block:**
  - The pin: `version` (if present), `strip_prefix`, `sha256`, `urls`.
  - The BUILD file path: from `build_file = "//deps:..."` if present, otherwise the `"BUILD.bazel"` key in `files = {...}`. Both can coexist. Resolve the `//deps:...` label to a filesystem path.
  - The overlay extras from `files = {...}`: checked-in config header(s) (`"config.h"`, `"config-linux-x86_64.h"`) and generated source lists (`"lib_contents.bzl"`). Steps 6 and 7 edit these.
  - The patch list and strip level: `patches = [...]` plus `patch_args = ["-pN"]` or `patch_strip = N` (check both forms). Input to Step 8.
- **Mode detection** — run `git diff main -- deps/<dep-name>/*.MODULE.bazel deps/repos.MODULE.bazel release.json`:
  - Any of those changed `<dep-name>`'s `version`/`sha256` → **Renovate mode**, skip Step 2.
  - Otherwise → **Manual mode**, run Step 2.
- **Sibling artefacts:** check for a `config_opts.txt` near the dep (legacy autotools flags; Step 5 input then delete).

---

## Step 2 — Bump the MODULE.bazel pin (manual mode only)

Skip if Step 1 detected Renovate mode. Otherwise read **[bump-version.md](bump-version.md)** and follow it.

---

## Step 3 — Refresh BUILD-file version-bearing fields

Runs in **both** modes — Renovate updates the pin but not the BUILD file's own version strings.

- Upstream package version: top-level `VERSION = "..."` constants and `expand_template` substitutions (`@VERSION@`, `@VERSION_NUMBER@`). If a top-level version constant exists, `expand_template` substitution values must reference it (`"@VERSION@": VERSION`) rather than duplicating the string.
- SO version: the `version = "..."` argument on `dd_cc_packaged` / `cc_shared_library` (names `lib<name>.so.X.Y.Z`). For autotools deps, derive from `configure.ac`'s `LIB<NAME>_LT_CURRENT/_AGE/_REVISION` via Linux libtool's `(CURRENT - AGE).AGE.REVISION`. **Use `configure.ac`, not the bundled `configure`** — tarballs occasionally ship a stale `configure`. For CMake deps, read `SOVERSION` / `VERSION` from `CMakeLists.txt`.

---

## Step 4 — Download both versions for diffing

- Download the **old** tarball (URL from MODULE.bazel), verify against the pinned sha256. Mismatch → stop.
- Download the **new** tarball, verify against the sha256 from Step 2 (or Renovate's pin). Mismatch → stop.
- Extract to `/tmp/<dep-name>-old/<dep-name>-<old-version>/` and `/tmp/<dep-name>-new/<dep-name>-<new-version>/`.

---

## Step 5 — Reconcile features, copts, and defines

Features gate everything downstream (sources, config-header `HAVE_*`/`USE_*`, preprocessor symbols). Land this first.

**Anchor on the config-header template**, not `configure.ac` — the template (`*.h.in` from `autoheader`, or with `#cmakedefine` for cmake) enumerates every define; `configure.ac` macros are scattered and conditional. The config-header name is not always `config.h`; resolve the actual name first.

### 5.1 Resolve the config-header name and template

- **Autotools:** grep `configure.ac` (or `configure` if `.ac` is missing) for `AC_CONFIG_HEADERS([...])` / `AC_CONFIG_HEADER([...])` / `AM_CONFIG_HEADER([...])`. The argument is the output header name; the template is that name + `.in`. Handle multiple headers.
- **CMake:** grep `CMakeLists.txt` recursively for `configure_file(<src>.in <dst> ...)` whose `<dst>` is a build header. The `<src>.in` is the template.
- Cross-check: the template name must exist in the upstream tree, and the output name should match a file checked in under `deps/<dep-name>/...`.
- If no template, fall back to reading `configure.ac` directly.

### 5.2 Capture current state

- `copts` and `LOCAL_DEFINES` from the Step 1 BUILD file.
- Any `config_opts.txt` from Step 1 — historical autotools flags. Once the BUILD file reflects the same decisions, **delete the `config_opts.txt` file**.

### 5.3 Diff the resolved template(s)

Diff the template between `/tmp/<dep-name>-old/.../template.in` and `/tmp/<dep-name>-new/.../template.in`. If renamed, resolve again on the new tree first.

Categorise each changed line:

| Change | Action |
|---|---|
| **Added** define (e.g. new `#undef HAVE_CLOCK_GETTIME`) | grep upstream `configure.ac`/`m4/` for the gating macro: `AC_CHECK_FUNCS` / `AC_CHECK_HEADERS` / `AC_CHECK_LIB` / `PKG_CHECK_MODULES` / `AC_ARG_ENABLE` / `AC_ARG_WITH`. Classify as **function check**, **header check**, **library check**, or **feature toggle**. |
| **Removed** define | Look in our current config header; if set, plan to drop. |
| **Renamed** define | Match by gating macro to keep continuity. |

### 5.4 Probe new function/header/library detections through the project's toolchain

Don't guess `HAVE_*` from upstream defaults — availability depends on our ctng toolchain's glibc baseline, and many functions are glibc-gated. A wrong answer links on the build host and fails on the deployment baseline.

**Probe mechanism** — under `/tmp/<dep-name>-probe/`, generate a Bazel package with one target per probe:

```python
# /tmp/<dep-name>-probe/BUILD.bazel
load("@rules_cc//cc:defs.bzl", "cc_binary")

cc_binary(
    name = "have_clock_gettime",
    srcs = ["have_clock_gettime.c"],
    target_compatible_with = ["@platforms//os:linux"],
)
```

```c
/* /tmp/<dep-name>-probe/have_clock_gettime.c — mirrors autotools' compile test */
#include <time.h>
int main(void) {
    struct timespec ts;
    return clock_gettime(CLOCK_REALTIME, &ts);
}
```

Probe patterns by check type:

- `AC_CHECK_FUNCS([foo])` → reference `foo` from `main()` after `#include` of its header. Build → `HAVE_FOO 1`.
- `AC_CHECK_HEADERS([foo/bar.h])` → `#include <foo/bar.h>` with trivial `main()`. Compile → `HAVE_FOO_BAR_H 1`.
- `AC_CHECK_LIB([foo], [bar])` → call `bar()` from `main()`, link the library. If it's a Bazel dep, depend on it through the same target.

Add the probe directory to the workspace temporarily. Run all probes in **one** `bazel build` (resolve the config flag from `.bazelrc`):

```
bazel build --config=<linux-config> //...
```

- Build success → define is `1`.
- Build failure with expected diagnostic (unresolved symbol, missing header) → define is undefined.
- Any other failure (toolchain unavailable, infra error) → stop the step, surface to the user. **Never guess on probe failure.**

Probe sources stay under `/tmp/`, not committed.

### 5.5 macOS and Windows caveat

For deps producing a darwin or windows config header:

- If the toolchain has a matching variant, probe through it the same way.
- Otherwise mark the relevant feature-table rows **"manual decision required"** and don't flip anything. Do not carry a Linux probe result to a different libc/CRT.

### 5.6 Build the feature decisions table

Surface a table to the user. Example shape:

```
Feature                              Current      Upstream default    Probe (linux)    Recommendation
-----------------------------------  -----------  ------------------  ---------------  ---------------
HAVE_CLOCK_GETTIME                   not present  yes                 ok               set to 1
HAVE_SYS_POLL_H                      1            (removed)           n/a              drop
USE_NGHTTP3 (opt-in feature)         off          off (default off)   n/a              keep off
HAVE_FOO_BAR_H (darwin variant)      1            yes                 not probed       manual decision required
```

Recommendation column:

- **Probe-backed rows:** the probe's answer.
- **Opt-in features** (`AC_ARG_ENABLE` / `AC_ARG_WITH` / cmake `option()`): "keep current".
- **Unprobed platforms:** "manual decision required".

Ask the user (one `AskUserQuestion`) which rows to override, then proceed with the agreed set.

### 5.7 Apply and carry forward

Apply the agreed copts / defines edits to the Step 1 BUILD file. Record the feature decisions for Steps 6 and 7. If a `config_opts.txt` existed and the BUILD file now reflects the same decisions, delete it.

---

## Step 6 — Reconcile sources (driven by Step 5 decisions)

1. Read upstream's `Makefile.am` / `CMakeLists.txt`. Map source files → gating feature.
2. For each currently-listed source: drop it if it no longer exists in the new tree, or if its gating feature was turned off in Step 5.
3. For new upstream files not in our list, classify as **gated** (under `if HAVE_FOO` in `Makefile.am`, `if(USE_FOO)` in cmake, `AM_CONDITIONAL`-guarded) or **ungated** (always compiled):
   - Ungated → include.
   - Gated, feature enabled in Step 5 → include.
   - Gated, feature disabled in Step 5 → skip.
4. Surface a combined diff (additions + removals + reasons), confirm with the user, apply.
5. If the BUILD file pulls its source list from a generated `.bzl` (e.g. `load(":lib_contents.bzl", ...)`), edit that `.bzl` in place. Keep its `# Generated by configure2bazel. Do not edit.` header.

---

## Step 7 — Refresh the config header

If the dep has a checked-in config header, read **[config-h-refresh.md](config-h-refresh.md)** and follow it. The Step 5 feature decisions are its input.

---

## Step 8 — Refresh patches

If the Step 1 http_archive block has a non-empty `patches = [...]`, read **[patches.md](patches.md)** and follow it. Use that list as the patch inventory and the block's `patch_args` for strip levels — don't glob the filesystem.

---

## Step 9 — Verify

Read **[verification.md](verification.md)** and follow it.

---

## Step 10 — Done

- Summarise the change set: one line per file from `git diff --stat`.
- Renovate branch: suggest amending the Renovate commit (`git commit --amend --no-edit`).
- Fresh branch: prompt for a commit message. Subject only.
- Hand off to `/create-pr` (it owns title prefix, labels, codex pre-review, release-note reminder, draft default). Don't call `gh pr create` directly.
- **Do not auto-commit or auto-push.**

---

## When to stop and ask

Stop and surface in these cases:

- Hash mismatch between downloaded bytes and the trusted pin.
- A trusted sha256 source could not be reached *and* the user didn't supply one.
- A probe failed for a non-availability reason (toolchain unavailable, infra error).
- A patch conflicts and `git apply --3way` can't resolve it.
- An `AC_ARG_ENABLE` / `AC_ARG_WITH` feature is newly introduced and the user hasn't picked a state.
- A build gate fails in a way the error-class table doesn't route cleanly.

Tell the user what happened, what you tried, and what input would unblock you. Do not guess past the failure.
