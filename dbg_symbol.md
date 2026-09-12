# ABLD-464: Debug-symbol ("dbg") packages for Bazel builds

Context and design record for [ABLD-464](https://datadoghq.atlassian.net/browse/ABLD-464)
("Process for creating 'dbg' packages"). Kept alongside the code so the rationale
survives past the PR description.

## Problem

Omnibus produces stripped release binaries plus a separate "-dbg" package/archive
containing the removed debug symbols, for post-mortem debugging of crash dumps from
stripped production binaries. Bazel packaging (`packages/`, built on `rules_pkg`) has
no equivalent yet — `packages/AGENTS.md` and `packages/installer/MIGRATION_PLAN.md`
both flag this as an open gap.

Requirements from the ticket:
- A normal `bazel build` of a target keeps debug symbols (dev/test use).
- A packaging build produces stripped binaries.
- A sibling packaging target produces *only* the debug symbols, no other files.
- No shadow dependency tree duplicated top-down to every binary/library target.
- Works for Go, C, and Rust objects; works on Linux, macOS, Windows.
- Callable from `rules_pkg` `pkg_files` construction.

## Two strategies, two PRs

The ticket names two candidate strategies. Rather than pick one up front, we
implemented both as independent PRs in separate worktrees of this repo, so they can be
compared in review and in CI before committing to one for the real migration:

- **Plan A — provider/rule-based** (`/Users/tony.aiuto/datadog-agent`, branch
  `aiuto/454-a`): a new `dd_strip_debug` rule wraps each binary/library's own build
  rule directly, adding a `DdStripInfo` provider and
  `OutputGroupInfo(stripped=..., debug=...)`. No aspect is needed for the core case —
  the default output stays unstripped, and a `pkg_files` transform just checks whether
  its `srcs[i]` carries `DdStripInfo`.
  See `bazel/rules/dd_strip/`, `bazel/toolchains/dd_strip/`,
  `bazel/rules/dd_packaging/dd_pkg_files_stripped.bzl`.
- **Plan B — packaging-time filter** (this repo, branch `aiuto/454-b`): a new
  `dd_pkg_strip_transform` rule intercepts files as they're
  assembled into a `pkg_files`/`PackageFilesInfo` tree and strips/splits them there,
  with no cooperation needed from the binary's own rule. This is what lets it work
  directly against today's `prebuilt_file.bzl`-backed product binaries
  (`@agent_binary//:agent`, etc.), which aren't real Bazel `go_binary`/`cc_binary`
  targets yet — Plan A can't wrap a provider around those until that migration lands.

Both use the same platform semantics, matching omnibus's `Stripper`
(`omnibus-ruby/lib/omnibus/stripper.rb`):
- **Linux**: `objcopy --only-keep-debug` → `.dbg`, `strip --strip-debug
  --strip-unneeded` in place, `objcopy --add-gnu-debuglink`.
- **macOS**: `strip` the shipped copy, `dsymutil` produces a `.dSYM` bundle. No prior
  art for this in omnibus or this repo (omnibus skips stripping on macOS entirely) —
  flagged as needing build-team confirmation before either PR merges.
- **Windows**: debug artifact = the *unstripped original* binary (not split DWARF),
  matching omnibus's `windows_symbol_stripping_file` semantics, since this repo's
  Go/Rust/mingw toolchains don't reliably produce standalone PDBs.

Both PRs independently build their own copy of the `bazel/toolchains/dd_strip`
toolchain rather than one depending on the other (explicit choice, made so both could
start immediately) — deduping them is a deliberate follow-up once one strategy is
chosen.

Full original design doc (file lists, verification steps, trade-off analysis):
`/Users/tony.aiuto/.claude/plans/we-are-going-to-flickering-pizza.md` (local to the
machine this was planned on, not checked in — this file is the durable summary).

## Known issue: macOS codesign ordering

`dd_cc_packaged` (`bazel/rules/dd_packaging/dd_cc_packaged.bzl`) runs `dd_strip_debug`
**after** `rewrite_rpath`. `rewrite_rpath`'s macOS implementation
(`bazel/toolchains/rpath_rewriter/rewrite_with_install_name_tool.sh`) ends with:

```sh
# Re-sign with an ad-hoc signature after modification as install_name_tool invalidates
# any existing code signature.
/usr/bin/codesign --sign - --force "$OUTPUT"
```

`install_name_tool` invalidates any existing signature, so `rewrite_rpath` re-signs
ad-hoc as its last step. `dd_strip_debug` then runs `strip`/`objcopy`/`dsymutil` on
that *already re-signed* output — and stripping/objcopy also invalidates a Mach-O code
signature, but **`dd_strip_debug` does not re-sign afterward**. The result: on macOS,
the final `.stripped` binary that actually ships out of `dd_cc_packaged` carries a
stale/invalid ad-hoc signature.

This was chosen deliberately to match omnibus's ordering ("strip is the last finalize
step"), but omnibus's ordering rationale is Linux-centric (objcopy debuglink chains)
and doesn't account for macOS's codesign-after-every-mutation requirement.

Two ways to fix, not yet decided:
1. Add a `codesign --sign - --force` re-sign step to `dd_strip_debug`'s macOS driver,
   after the strip step, so it always leaves a valid ad-hoc signature — keeps
   omnibus's "strip last" ordering.
2. Reorder so `dd_strip_debug` runs *before* `rewrite_rpath` on macOS specifically,
   since `rewrite_rpath` already re-signs as its last step — avoids adding a second
   codesign invocation, but only applies to the `dd_cc_packaged` call chain (Plan A);
   for Plan B's flattened packaging-time model there's no equivalent single choke
   point to reorder around, so option 1 (re-sign after strip) is likely the more
   portable fix across both PRs.

This affects Plan A directly (found during its implementation/testing). Plan B should
be checked for the same issue if/when it starts producing real signed macOS binaries
through `dd_cc_packaged`-style consumers.

## Other open items (both PRs)

- Linux `objcopy` path is unverified locally — both PRs were implemented and tested on
  macOS arm64 with no Linux exec platform available; needs CI or a Linux sandbox.
- `packages/agent/product/BUILD.bazel`'s real binaries come from `prebuilt_file.bzl`
  (built by `dda`, not a real Bazel target) — Plan A can only demonstrate against
  `//cmd/agent:agent` and `rtloader` directly, not the full product bundle, until that
  migration lands. Plan B has no such blocker.
- Plan A: `dsymutil` produced an empty `.dSYM` for at least one pure-Go binary during
  testing — needs investigation (Go's DWARF layout vs. dsymutil's expectations).
- Whatever builds release binaries outside Bazel (`dda inv agent.build` / omnibus)
  must not pre-strip them, or there's nothing left for either strategy to split.
  Out of scope for both PRs; flagged as a cross-cutting dependency.
