# Config-header refresh

This doc expands Step 7 of the main flow. It assumes Step 5 ran and you already have:

- The resolved config-header **template** path.
- The resolved **output header** path(s) we have checked in.
- The **feature decisions table** with probe results — function/header/library `HAVE_*` decisions are already resolved at high confidence; opt-in feature toggles either have an explicit user choice or are marked manual-decision-required.

This step does three things, in order: build the upstream-changes / impact summary, get user approval, apply the edits.

---

## What this step does NOT do

- **Re-probe features.** Probes ran in Step 5. If a HAVE_* is in the decisions table, use the value there.
- **Run upstream configure / cmake.** Out of scope for this skill version. See the "real-configure fallback" note at the bottom if the user explicitly asks for higher confidence.
- **Silently flip feature defines.** Every change is in the summary, and the user approves before edits land.

---

## Build the summary

The summary has two sections. **Print it before any edit.** Even a "trivial" version-only bump prints both sections so it's obvious nothing was skipped.

```
## Upstream build-system changes (<old-version> → <new-version>)

  <template path>
    + <new template line>                → <classification>
    - <removed template line>            → <classification>
    ~ <changed template line>            → <classification>
  configure.ac (or CMakeLists.txt)
    + AC_ARG_ENABLE([nghttp3], …)        → new optional feature (off by default)
    ~ AC_INIT([curl], [8.20.0], …)       → version 8.19.0 → 8.20.0
  <other relevant files>
    (none)

## Impact on <output config-header path(s)>

  version & build-identifier defines (auto-applied):
    LIBCURL_VERSION         "8.19.0" → "8.20.0"
    LIBCURL_VERSION_NUM     0x081300 → 0x081400
    PACKAGE_STRING          "curl 8.19.0" → "curl 8.20.0"
    <FOO_BUILD_ID>          "<old>" → "<new>"      (existing, populated)
  build-identifier opt-outs preserved:
    <FOO_OPT_OUT>           "" or "<placeholder>"  (prior choice; not reversed)
  new build-identifier defines (decision required):
    <FOO_NEW_BUILD_ID>      ? → first-time choice
  new defines to add (from Step 5 decisions):
    HAVE_CLOCK_GETTIME      1        [probe: ok on ctng-linux]
  defines to remove:
    HAVE_SYS_POLL_H                  (header check removed upstream)
  defines requiring manual decision:
    USE_NGHTTP3             ?        (opt-in; new in upstream — user must choose)
    HAVE_FOO_BAR_H (darwin) ?        (darwin probe not available; user must choose)
  unchanged:
    <N> other HAVE_*/USE_* defines kept as-is
```

For **multi-platform configs** (curl `linux/`+`darwin/`, unixodbc `x86_64/`+`aarch64/`, etc.), repeat the "Impact on …" section once per platform variant. Header availability and `HAVE_*` defaults can differ across libcs/CRTs/arch baselines; don't merge the variants into one section.

---

## Classifier: template line → upstream macro → classification

Use template entries as the primary input; the corresponding configure macro is looked up only to label the entry.

| Template line | Look up in upstream | Classification |
|---|---|---|
| `#undef HAVE_FOO_BAR_H` | `AC_CHECK_HEADERS([foo/bar.h])` in `configure.ac` / `m4/*.m4` | header check |
| `#undef HAVE_FOO` (function-shaped name) | `AC_CHECK_FUNCS([foo])` / `AC_CHECK_FUNC([foo], …)` | function check |
| `#undef HAVE_LIBFOO` | `AC_CHECK_LIB([foo], [bar])` | library check |
| `#undef HAVE_FOO` corresponding to a `pkg-config` module | `PKG_CHECK_MODULES([FOO], [foo])` | library check (pkg-config) |
| `#undef PACKAGE_VERSION` / `VERSION` / `PACKAGE_STRING` / `<NAME>_VERSION_NUM` | `AC_INIT(…)` / `m4_define([VERSION], …)` | version-like (auto-applied) |
| `#undef BUILD_COMMITID` / `BUILD_REVISION` / `BUILD_HOSTNAME` / similar build-identifier macros | `AC_DEFINE_UNQUOTED([NAME], ["$var"], …)` where `$var` is set from a tarball-bundled file (often a `VERSION` file at the tarball root) or `m4_esyscmd` output | build identifier (auto-applied from the tarball) |
| `#undef ENABLE_FOO` / `#undef USE_FOO` | `AC_ARG_ENABLE([foo], …)` / `AC_ARG_WITH([foo], …)` / cmake `option(FOO …)` | feature toggle (manual choice) |
| `#define FOO <literal>` (verbatim line in template, no `#undef`) | `AC_DEFINE([FOO], [<literal>], …)` | literal define (carry over) |
| `#cmakedefine FOO` | `option(FOO …)` or `set(FOO …)` in CMakeLists | cmake feature toggle |

If the grep finds no matching macro for a new template line, log the orphan template entry to the user — usually means an `m4` macro was renamed and you need to look further.

---

## Apply

1. **Version-like and build-identifier defines:** auto-apply, propagating verbatim to **each** platform variant.
   - Version-like (`PACKAGE_VERSION`, `PACKAGE_STRING`, `VERSION`, and `<NAME>_VERSION_NUM` / `_MAJOR` / `_MINOR` / `_PATCH` / `_STRING`): take from `AC_INIT` or `m4_define([VERSION], …)`. Compute numeric versions from the upstream's own conversion (often `(major << 16) | (minor << 8) | patch`, but check the prior value — every project picks its own encoding).
   - Build identifiers (`BUILD_COMMITID`, `BUILD_REVISION`, etc.): trace each `AC_DEFINE_UNQUOTED` chain in `configure.ac` to its source value. Common sources are tarball-bundled metadata files (often a `VERSION` file at tarball root carrying the commit id), `m4_esyscmd` outputs evaluable at unpack time, or string literals in configure.ac. Read the value from the tarball and apply it — **except**:
     - **Preserve existing empty or placeholder values.** If our config header has the define set to `""` (empty) or a fixed placeholder like `"<none>"`, leave it alone. An empty value is a deliberate opt-out (we don't want that identifier baked into the shipped binary); auto-applying the tarball's value would silently reverse that policy.
     - **New build-identifier defines** introduced in this bump (not in our previous config header) default to `""`. Surface them in the summary so the user can opt into populating them — but the safe default is opt-out, since populating commits a value into binary diagnostics that downstream may then depend on.
     - **Configure-time-only values** (timestamp, hostname, configure command line) keep the existing placeholder — Bazel doesn't reproduce them, and a placeholder is more honest than a stale or fake value.
2. **Probe-backed defines (Step 5 results):** apply per platform variant where probes ran. For unprobed variants the row is marked "manual decision required" and is **not** auto-applied.
3. **Removed defines:** delete from each platform variant.
4. **Opt-in features:** apply only the choice the user already made in Step 5. If the user hasn't picked, stop and ask before proceeding to the next variant.

Preserve the file's existing header comment (`/* config.h. Generated from config.h.in by configure. */`) — it's accurate as a description of how the file was first created. The skill is patching it, not regenerating from scratch.

---

## Post-apply check

After editing each variant:

- Spot-check that the file is still C-valid: `bazel build @<dep-name>//...` can be deferred to Step 9, but a quick `cpp -fsyntax-only` against the file is a cheap sanity check if the upstream tree is in `/tmp/` and headers are reachable. Skip if it's awkward — Step 9 will catch it.
- Confirm that the count of `#define HAVE_*` in the new file matches the expectation derived from the decisions table. A large unexpected delta usually means a `#undef`/`#define` was missed.

If anything looks wrong, stop and re-show the summary with the discrepancy highlighted; do not silently re-edit.

---

## Real-configure fallback (out of scope by default)

If the user explicitly asks for higher confidence than build-system-diff + probes can give, the fallback is to run the upstream `./configure` (or `cmake`) inside the project's toolchain and copy the resulting header. That requires the dep's build-time deps to be available and the same compiler / sysroot the rest of the build uses. It is **not** automated by this skill — point the user at the relevant per-dep notes in the upstream's INSTALL/README and let them do the manual rebuild.
