---
name: update-3rd-party-libs
description: Update a C/C++ third-party library under deps/ — bump version, reconcile sources/copts/config.h/patches, and verify the build.
argument-hint: "<dep-name> [target-version]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, AskUserQuestion
---

## Overview

Update one C/C++ dep under `deps/` end-to-end. After Bazel-ification, a version bump is no longer just "new sha256" — the source listing, copts/defines, the checked-in config header, and any patches all have to be reconciled against the new upstream tarball. This skill walks through that reconciliation for a single dep.

**Arguments:** `$ARGUMENTS` — `<dep-name>` (required), `<target-version>` (optional; the skill will ask if not supplied).

Scope: C/C++ deps under `deps/` that have a Bazel overlay. **Out of scope:** openssl, python, krb5 (no overlay), Go deps, Python packages.

---

## Operating principles

- **Never produce a pin-checksum from an unverified download.** Pin sha256 from a trusted upstream source (`.sha256`/`SHA256SUMS`, signed manifest, GPG-verified tarball) or ask the user. Local hashing is for *verifying* against an existing trusted value, not producing one.
- **Probe `HAVE_*` detections with the project's Bazel toolchain.** Linux uses a ctng glibc baseline; guessing from upstream defaults is wrong for glibc-gated functions.
- **Surface, don't silently flip.** Every non-version `HAVE_*`/`USE_*` decision goes through the user before any edit.
- **Stop on real failures.** Hash mismatch, probe failure, patch conflict — surface and pause; never guess past an error.

---

## Supporting documents

| When you need to...                                          | Read this                                            |
|--------------------------------------------------------------|------------------------------------------------------|
| Bump the MODULE.bazel pin (manual mode only)                 | [bump-version.md](bump-version.md)                   |
| Refresh the checked-in config header                         | [config-h-refresh.md](config-h-refresh.md)           |
| Refresh patches against the new source tree                  | [patches.md](patches.md)                             |
| Run the build gates and route failures back to the right step | [verification.md](verification.md)                  |

**Do not load all docs at once.** Read each only when you reach the step that references it.

---

## Step 1 — Locate the dep and detect mode

Resolve everything from the dep's `http_archive` block. Filesystem layout varies (some deps live entirely at the `deps/` root, others under `deps/<dep>/`, with or without an `overlay/` subdir) — the MODULE.bazel block is the canonical source of truth.

- **Find the http_archive block.** Look in `deps/repos.MODULE.bazel`, then in `deps/<dep-name>/<dep-name>.MODULE.bazel` if present. For deps using `module_utils.bzl` (read by `use_repo_rule`), the equivalent state is in `release.json` under `<NAME>_VERSION` / `<NAME>_SHA256`. If you can't find any of these, stop — the dep is one of the special cases (openssl, python, krb5) or doesn't have a Bazel overlay yet.
- **Capture from the http_archive block:**
  - The pin: `version` (if present), `strip_prefix`, `sha256`, `urls`.
  - The BUILD file path: comes from `build_file = "//deps:..."` if present, otherwise the `"BUILD.bazel"` key in `files = {...}`. Both can coexist on the same `http_archive` — `build_file` provides the BUILD file directly, `files` carries additional overlay artefacts on top. Resolve the `//deps:...` label to a filesystem path.
  - The overlay extras from `files = {...}` (when present). Common entries: checked-in config header(s) like `"config.h"` / `"config-linux-x86_64.h"`, and generated source lists like `"lib_contents.bzl"`. These are the artefacts Steps 6 and 7 will edit.
  - The patch list and strip level: `patches = [...]` plus either `patch_args = ["-pN"]` or `patch_strip = N` (both forms exist in `http_archive`; check both). Used as the authoritative patch inventory and as input to Step 8.
- **Mode detection** — run `git diff main -- deps/<dep-name>/*.MODULE.bazel deps/repos.MODULE.bazel release.json`:
  - If any of those files have changes that touch `<dep-name>`'s `version`/`sha256` → **Renovate mode**, skip Step 2.
  - Otherwise → **Manual mode**, run Step 2.
- **Sibling artefacts in the filesystem** (the MODULE.bazel block already enumerated most of them via `files`/`patches`; check for one straggler):
  - A `config_opts.txt` near the dep (legacy autotools flags; treat as Step 5 input then delete).

---

## Step 2 — Bump the MODULE.bazel pin (manual mode only)

Skip if Step 1 detected Renovate mode (Renovate already did this).

Otherwise, read **[bump-version.md](bump-version.md)** and follow it.

---

## Step 3 — Refresh BUILD-file version-bearing fields

Runs in **both** modes — Renovate updates the http_archive pin but not the BUILD file's own version strings.

- Upstream package version: top-level `VERSION = "..."` constants and `expand_template` substitutions (`@VERSION@`, `@VERSION_NUMBER@`).
- SO version: the `version = "..."` argument on `dd_cc_packaged` / `cc_shared_library` (names `lib<name>.so.X.Y.Z`). For autotools deps, derive from `configure.ac`'s `LIB<NAME>_LT_CURRENT/_AGE/_REVISION` via Linux libtool's `(CURRENT - AGE).AGE.REVISION`. **Use `configure.ac`, not the bundled `configure`** — `.ac` is the source of truth; tarballs occasionally ship a stale `configure` because upstream forgot to `autoreconf`. For CMake deps, read `SOVERSION` / `VERSION` from `CMakeLists.txt`.

---

## Step 4 — Download both versions for diffing

Steps 5-8 diff old vs new source trees.

- Download the **old** tarball (URL from MODULE.bazel), verify bytes match the pinned sha256. Mismatch → stop (mirror drifted or pin is wrong).
- Download the **new** tarball, verify against the sha256 from Step 2 (or Renovate's pin). Mismatch → stop.
- Extract to `/tmp/<dep-name>-old/<dep-name>-<old-version>/` and `/tmp/<dep-name>-new/<dep-name>-<new-version>/`. No further network needed.

---

## Step 5 — Reconcile features, copts, and defines

Features gate everything downstream: sources (Step 6), `HAVE_*`/`USE_*` in the config header (Step 7), preprocessor symbols. Land this first.

**Anchor on the config-header template**, not on `configure.ac`. The template (`*.h.in` from `autoheader`, or `*.h.in` with `#cmakedefine` for cmake) enumerates every possible define; `configure.ac` macros are scattered and conditional.

**The config-header name is not always `config.h`.** Resolve the actual name first.

### 5.1 Resolve the config-header name and template

- **Autotools:** grep `configure.ac` (and `configure` if `.ac` is missing) for `AC_CONFIG_HEADERS([...])` / `AC_CONFIG_HEADER([...])` / `AM_CONFIG_HEADER([...])`. The argument is the output header name; the template is the same name with `.in` appended. Handle multiple headers.
- **CMake:** grep `CMakeLists.txt` (recursively) for `configure_file(<src>.in <dst> ...)` lines whose `<dst>` is a header used by the build. The `<src>.in` is the template.
- Cross-check: the resolved template name must exist as a real file in the upstream tree, and the resolved output name should correspond to a file we have checked in under `deps/<dep-name>/...`.
- If no template is present, fall back to reading `configure.ac` directly.

### 5.2 Capture current state

Pull from the dep:

- `copts` and `LOCAL_DEFINES` from the BUILD file resolved in Step 1 (from the `files = {"BUILD.bazel": "..."}` mapping).
- Any `config_opts.txt` from Step 1's inventory — historical autotools flags. Treat as additional input here; once the BUILD file accurately reflects the same decisions, **delete the `config_opts.txt` file** as part of this run.

Together these describe the feature decisions we have made for this dep.

### 5.3 Diff the resolved template(s)

Diff the template between `/tmp/<dep-name>-old/.../template.in` and `/tmp/<dep-name>-new/.../template.in`. If the template was renamed between versions, resolve again on the new tree before diffing.

Categorise each changed line:

| Change | Action |
|---|---|
| **Added** define (e.g. new `#undef HAVE_CLOCK_GETTIME`) | grep upstream `configure.ac`/`m4/` for the macro that gates it: `AC_CHECK_FUNCS` / `AC_CHECK_HEADERS` / `AC_CHECK_LIB` / `PKG_CHECK_MODULES` / `AC_ARG_ENABLE` / `AC_ARG_WITH`. Classify as **function check**, **header check**, **library check**, or **feature toggle**. |
| **Removed** define | Look in our current config header; if set, plan to drop. |
| **Renamed** define | Match by gating macro to keep continuity. |

### 5.4 Probe new function/header/library detections through the project's toolchain

Don't guess `HAVE_*` from upstream defaults. Availability depends on our ctng toolchain's glibc baseline — many functions (`pidfd_open`, `statx`, `copy_file_range`, newer `clock_*`) are glibc-gated. A wrong answer produces a binary that links on the build host and fails on the deployment baseline.

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

- `AC_CHECK_FUNCS([foo])` → reference `foo` from `main()` (after `#include` of its declaring header). Build → `HAVE_FOO 1`.
- `AC_CHECK_HEADERS([foo/bar.h])` → `#include <foo/bar.h>` with a trivial `main()`. Compile → `HAVE_FOO_BAR_H 1`.
- `AC_CHECK_LIB([foo], [bar])` → call `bar()` from `main()`, link against the library. If the library is itself a Bazel dep, depend on it through the same target.

Add the probe directory to the workspace temporarily (write a small `MODULE.bazel`-snippet that includes it, or invoke from a transient workspace under `/tmp/`). Run all probes in **one** `bazel build` invocation so Bazel batches the work:

```
bazel build --config=<linux-config> //...
```

(resolve the right config flag from `.bazelrc`.)

- Build success → define is `1`.
- Build failure with expected diagnostic (unresolved symbol, missing header) → define is undefined.
- Any other failure (toolchain unavailable, infra error) → stop the step, surface to the user. **Never guess on probe failure.**

Probe sources stay under `/tmp/` and are not committed.

### 5.5 macOS and Windows caveat

We build very little of `deps/` for those platforms with Bazel today (curl has both, freetds/unixodbc are Linux-only, etc.). For deps producing a darwin or windows config header:

- If the project's toolchain has a matching variant for that platform, probe through it the same way.
- Otherwise, mark the relevant rows in the feature table as **"manual decision required"** and stop short of flipping anything. Do not carry the Linux probe result over to a different libc/CRT.

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

Apply the agreed copts / defines edits to the BUILD file resolved in Step 1. Record the feature decisions in memory for Steps 5 and 6.

If a `config_opts.txt` existed and the BUILD file now reflects the same decisions, delete it.

---

## Step 6 — Reconcile sources (driven by Step 5 decisions)

After features, because upstream gates source files on them.

1. Read upstream's `Makefile.am` / `CMakeLists.txt`. Map source files → gating feature.
2. For each currently-listed source: drop it if it no longer exists in the new tree, or if its gating feature was turned off in Step 5.
3. For new upstream files not in our list, classify as **gated** (under `if HAVE_FOO` in `Makefile.am`, `if(USE_FOO)` in cmake, `AM_CONDITIONAL`-guarded lists) or **ungated** (always compiled):
   - Ungated → include.
   - Gated, feature enabled in Step 5 → include.
   - Gated, feature disabled in Step 5 → skip.
4. Surface a combined diff (additions + removals + reasons), confirm with the user, apply.
5. If the BUILD file pulls its source list from a generated `.bzl` (e.g. via `load(":lib_contents.bzl", ...)` — the file will be one of the entries in the http_archive `files = {...}` mapping), edit that `.bzl` file in place. Keep its `# Generated by configure2bazel. Do not edit.` header — accurate for humans (this skill is the regenerator now, but it's still not for hand-editing).

---

## Step 7 — Refresh the config header

If the dep has a checked-in config header, read **[config-h-refresh.md](config-h-refresh.md)** and follow it. The Step 5 feature decisions are its input.

---

## Step 8 — Refresh patches

If the http_archive block from Step 1 has a non-empty `patches = [...]` list, read **[patches.md](patches.md)** and follow it. Use the labels from that list as the patch inventory and the block's `patch_args` for strip levels — don't glob the filesystem.

---

## Step 9 — Verify

Read **[verification.md](verification.md)** and follow it.

---

## Step 10 — Done

- Summarise the change set: one line per file from `git diff --stat`.
- On a Renovate branch: suggest amending the Renovate commit (`git commit --amend --no-edit`) so the PR carries the full change set.
- On a fresh branch: prompt for a commit message. Subject only — no body that just restates the diff.
- Hand off to `/create-pr` for the PR (it owns title prefix, labels, codex pre-review, release-note reminder, draft default). Don't call `gh pr create` directly.
- **Do not auto-commit or auto-push.** The skill prepares the diff and stops.

---

## When to stop and ask

The skill should bias toward forward motion, but stop and surface in these cases:

- Hash mismatch between downloaded bytes and the trusted pin.
- A trusted sha256 source could not be reached *and* the user didn't supply one.
- A probe failed for a non-availability reason (toolchain unavailable, infra error).
- A patch conflicts and `git apply --3way` can't resolve it.
- An `AC_ARG_ENABLE` / `AC_ARG_WITH` feature is newly introduced and the user hasn't picked a state for it.
- A build gate fails in a way the error-class table doesn't route cleanly.

In each case: tell the user what happened, what you tried, and what input would unblock you. Do not proceed past the failure with a guess.
