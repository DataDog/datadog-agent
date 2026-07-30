# Config-header refresh

Expands Step 7. Assumes Step 5 ran and you have:

- The resolved config-header **template** path.
- The resolved **output header** path(s) checked in.
- The **feature decisions table** with probe results.

In order: build the upstream-changes / impact summary, get user approval, apply the edits.

---

## What this step does NOT do

- Re-probe features. Use the decisions-table value for any `HAVE_*`.
- Run upstream configure / cmake. See "real-configure fallback" below.
- Silently flip feature defines. User approves before edits land.

---

## Build the summary

Two sections. **Print it before any edit.** Even a version-only bump prints both sections.

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

For **multi-platform configs**, repeat the "Impact on …" section once per platform variant — `HAVE_*` defaults differ across libcs/CRTs/arch baselines. Don't merge variants.

---

## Classifier: template line → upstream macro → classification

Template entries are the primary input; the configure macro is looked up only to label the entry.

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

If the grep finds no matching macro for a new template line, log the orphan entry to the user — usually an `m4` macro was renamed.

---

## Apply

1. **Version-like and build-identifier defines:** auto-apply, propagating verbatim to **each** platform variant.
   - Version-like (`PACKAGE_VERSION`, `PACKAGE_STRING`, `VERSION`, `<NAME>_VERSION_NUM` / `_MAJOR` / `_MINOR` / `_PATCH` / `_STRING`): take from `AC_INIT` or `m4_define([VERSION], …)`. Compute numeric versions from the prior value's encoding (often `(major << 16) | (minor << 8) | patch`, but every project differs).
   - Build identifiers (`BUILD_COMMITID`, `BUILD_REVISION`, etc.): trace each `AC_DEFINE_UNQUOTED` chain in `configure.ac` to its source value (tarball-bundled metadata, `m4_esyscmd` output, or string literals), read it from the tarball, apply it — **except**:
     - **Preserve existing empty or placeholder values** (`""`, `"<none>"`). An empty value is a deliberate opt-out.
     - **New build-identifier defines** default to `""`; surface in the summary for the user to opt into populating.
     - **Configure-time-only values** (timestamp, hostname, configure command line) keep the existing placeholder — Bazel doesn't reproduce them.
2. **Probe-backed defines (Step 5 results):** apply per platform variant where probes ran. Unprobed variants are marked "manual decision required", **not** auto-applied.
3. **Removed defines:** delete from each platform variant.
4. **Opt-in features:** apply only the choice the user made in Step 5. If unpicked, stop and ask before the next variant.

Preserve the file's existing header comment (`/* config.h. Generated from config.h.in by configure. */`). The skill patches it, not regenerates it.

---

## Post-apply check

After editing each variant:

- Spot-check C-validity with `cpp -fsyntax-only` if the upstream tree is in `/tmp/` and headers reachable; otherwise defer to Step 9.
- Confirm the `#define HAVE_*` count matches the decisions table. A large delta means a `#undef`/`#define` was missed.

If anything looks wrong, stop and re-show the summary with the discrepancy highlighted; do not silently re-edit.

---

## Real-configure fallback (out of scope by default)

If the user explicitly asks for higher confidence than build-system-diff + probes give, run the upstream `./configure` (or `cmake`) inside the project's toolchain and copy the resulting header. Requires the dep's build-time deps and the same compiler / sysroot. **Not** automated by this skill — point the user at the upstream's INSTALL/README for the manual rebuild.
