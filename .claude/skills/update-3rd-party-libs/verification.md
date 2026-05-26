# Verification

This doc expands Step 9 of the main flow. Run after Steps 5-8 have applied all their edits.

The goal: prove the dep still builds in isolation **and** that nothing downstream broke.

---

## Gates (must pass both)

### Gate 1 — Build the dep's product targets

Identify the dep's actual *products* (the labels downstream consumers depend on) and build those. The cc_shared_library / cc_library / pkg_install rules at the top of the dep's BUILD file are usually it; for gcrypt that's `@gcrypt//:gcrypt`, `:libgcrypt`, `:install`.

```
bazel build @<dep-name>//:<product1> @<dep-name>//:<product2> ...
```

**Avoid `@<dep-name>//...` ("build all targets").** Many deps include intermediate cc_library targets that are platform-specific without a `target_compatible_with` constraint — those targets break on the wrong platform even though no real consumer depends on them. `:all` also exercises private test/helper binaries that aren't part of what the agent ships. Building the products only avoids both problems. (If you discover such a missing-constraint target during this run, treat it as a separate pre-existing issue — document it but don't block the bump on fixing it.)

For deps with a `BUILD.bazel` wrapper at `//deps/<dep-name>:...` (typically holding tests/wrappers), also build the things declared there:

```
bazel build //deps/<dep-name>/...
```

Why this gate first: catches obvious problems — missing source files, broken copts, unresolved internal includes — without dragging the rest of the agent into the failure.

### Gate 2 — Build the agent's "everything" bundle

```
bazel build //packages/agent/linux:everything
```

`:everything` is a `pkg_filegroup` aggregating every native artefact that goes into the Linux distribution. Building it forces every cc_library/cc_binary/cc_shared_library transitively reachable from the agent to compile and link, which is what catches downstream link errors (missing symbol, ABI mismatch, header drift) caused by the dep bump.

**Why `:everything` and not `:debian` or `:rpm`:** the distro packagers run `MakeDeb` / `MakeRpm` *on top of* compiling everything. That packaging step is slow (~minutes for the final pack), distro-format-specific, and doesn't add coverage relevant to a third-party-lib bump. `:everything` stops at "everything compiled and linked", which is the gate the bump actually needs to clear.

You can sanity-check the dep is in the bundle with:
```
bazel cquery 'somepath(//packages/agent/linux:everything, @<dep-name>//:<product>)'
```

If the path is empty, the dep isn't actually consumed by the Linux agent — choose a more targeted consumer (a specific package or test that depends on it directly).

---

## On failure: route to the right earlier step

Don't restart from scratch. Map the error class to the implicated step, fix, and re-verify.

| Failure class (example diagnostic) | Likely cause | Re-route to |
|---|---|---|
| `fatal error: '<header>' file not found` | Source list references a file Step 6 should have dropped, or includes a path Step 5's defines no longer enable | Step 6 (sources), then Step 5 (defines) |
| `undefined reference to '<symbol>'` linking the dep | A source file containing the symbol's definition was dropped in Step 6 | Step 6 (sources) |
| `undefined reference to '<symbol>'` linking the agent | The dep no longer exports a symbol the agent needs — usually a feature got turned off in Step 5 | Step 5 (defines) |
| `error: '<func>' undeclared` inside dep source | `HAVE_<FUNC>` was set in config.h but glibc baseline doesn't provide it (probe lied, or copy-paste error) | Step 5 probes, then Step 7 (config.h variant) |
| `redefinition of 'PACKAGE_VERSION'` etc. | Version macro updated in one variant of the config header but not all | Step 7 — re-check every platform variant |
| `error: invalid preprocessing directive` in config.h | Stray `#cmakedefine` left from cmake template, or autotools `#undef` wasn't substituted | Step 7 — re-check the substitution |
| `patch does not apply` during Bazel fetch | A patch that the patches step missed, or a regenerated patch that's malformed | Step 8 — re-run on this patch |
| `sha256 mismatch` during `http_archive` fetch | The pinned hash is wrong (likely the user pasted incorrectly, or Renovate's value drifted from upstream) | Step 2 — re-acquire from trusted source |

If a failure doesn't match any row in the table, **stop and surface the raw error** to the user. Don't guess past it.

---

## What this step does NOT do

- **Run `bazel test` blindly.** Only run tests if `deps/<dep-name>/BUILD.bazel` has obvious quick test targets — e.g. `cc_test` rules already declared. Large or slow test suites are out of scope here; Step 10 (Done) will note in the summary if there were test targets that weren't exercised, and the user can decide whether to run them before merging.
- **Skip a gate that's "probably fine".** If Gate 1 passes but Gate 2 errors mention a dep-specific symbol, the gates served their purpose — both have to pass.
- **Re-verify after a documented patch failure.** A patch that couldn't be refreshed (Step 8 documented it for human resolution) may or may not break the build. If it breaks, the failure surfaces here and the user gets the conflict report; the skill doesn't try to mask the impact.

---

## After both gates pass

Hand control back to Step 10 (Done). Summarise the change set there, not here.
