# Verification

Expands Step 9. Run after Steps 5-8 have applied all their edits. Goal: prove the dep still builds in isolation **and** that nothing downstream broke.

---

## Gates (must pass both)

### Gate 1 — Build the dep's product targets

Build the dep's *products* (the labels downstream consumers depend on) — usually the cc_shared_library / cc_library / pkg_install rules at the top of the dep's BUILD file.

```
bazel build @<dep-name>//:<product1> @<dep-name>//:<product2> ...
```

**Avoid `@<dep-name>//...`.** Intermediate cc_library targets are often platform-specific without a `target_compatible_with` constraint and break on the wrong platform even though no consumer depends on them; `:all` also exercises private test/helper binaries the agent doesn't ship. If you find such a missing-constraint target, document it as a pre-existing issue but don't block the bump on it.

For deps with a `BUILD.bazel` wrapper at `//deps/<dep-name>:...`, also build it:

```
bazel build //deps/<dep-name>/...
```

Gate 1 first: catches missing source files, broken copts, unresolved internal includes without dragging in the rest of the agent.

### Gate 2 — Build the agent's "everything" bundle

```
bazel build //packages/agent/linux:everything
```

`:everything` is a `pkg_filegroup` aggregating every native artefact in the Linux distribution; building it forces everything transitively reachable to compile and link, catching downstream link errors (missing symbol, ABI mismatch, header drift). Not `:debian`/`:rpm`: those add slow, distro-specific packaging with no coverage relevant to a lib bump.

Sanity-check the dep is in the bundle:
```
bazel cquery 'somepath(//packages/agent/linux:everything, @<dep-name>//:<product>)'
```

If the path is empty, the dep isn't consumed by the Linux agent — choose a more targeted consumer.

---

## On failure: route to the right earlier step

Map the error class to the implicated step, fix, and re-verify.

| Failure class (example diagnostic) | Likely cause | Re-route to |
|---|---|---|
| `fatal error: '<header>' file not found` | Source list references a file Step 6 should have dropped, or includes a path Step 5's defines no longer enable | Step 6 (sources), then Step 5 (defines) |
| `undefined reference to '<symbol>'` linking the dep | A source file containing the symbol's definition was dropped in Step 6 | Step 6 (sources) |
| `undefined reference to '<symbol>'` linking the agent | The dep no longer exports a symbol the agent needs — usually a feature got turned off in Step 5 | Step 5 (defines) |
| `error: '<func>' undeclared` inside dep source | `HAVE_<FUNC>` was set in config.h but glibc baseline doesn't provide it (probe lied, or copy-paste error) | Step 5 probes, then Step 7 (config.h variant) |
| `redefinition of 'PACKAGE_VERSION'` etc. | Version macro updated in one variant of the config header but not all | Step 7 — re-check every platform variant |
| `error: invalid preprocessing directive` in config.h | Stray `#cmakedefine` left from cmake template, or autotools `#undef` wasn't substituted | Step 7 — re-check the substitution |
| `patch does not apply` during Bazel fetch | A patch that the patches step missed, or a regenerated patch that's malformed | Step 8 — re-run on this patch |
| `sha256 mismatch` during `http_archive` fetch | The pinned hash is wrong (user pasted incorrectly, or Renovate's value drifted from upstream) | Step 2 — re-acquire from trusted source |

If a failure doesn't match any row, **stop and surface the raw error** to the user. Don't guess past it.

---

## What this step does NOT do

- Run `bazel test` blindly. Only run already-declared quick `cc_test` targets in `deps/<dep-name>/BUILD.bazel`; note un-exercised test targets in the Step 10 summary.
- Skip a gate. Both must pass.
- Re-verify after a documented patch failure (Step 8). If it breaks the build, the failure surfaces here.

---

## After both gates pass

Hand control back to Step 10. Summarise the change set there, not here.
