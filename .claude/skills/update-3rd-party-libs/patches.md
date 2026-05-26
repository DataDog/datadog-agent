# Patch refresh

This doc expands Step 8 of the main flow. Step 1 captured the `patches = [...]` list (resolved to filesystem paths) and the strip level (`patch_args` / `patch_strip`) from the http_archive block. This doc operates on that data, not on filesystem globs.

---

## Goal

For each patch in the dep's `patches = [...]` list, in order, against the new upstream tarball: confirm it still applies, or record that it doesn't. The skill detects drift; it doesn't rewrite patch files.

**Two-level policy:**

- **Within this step:** keep walking the patch list even after a failure, so one run surfaces every failed patch (not just the first).
- **Across the bump:** if any patch in the list failed, the bump halts here. Do not proceed to Step 9 (Verify) or Step 10 (Done). The user resolves the failed patches manually and re-runs.

---

## Procedure

Bazel applies the `patches = [...]` list cumulatively in order at fetch time — patch N sees the tree after patches 1..N-1. Mirror that by keeping one cumulative scratch tree per dep.

### 1. Resolve the strip level

`patch_args = ["-pN", ...]` or `patch_strip = N` from Step 1 is authoritative. If neither is present, Bazel's `http_archive` default is `-p0` (note: not `git apply`'s default of `-p1`). Verify by checking each patch's `---` / `+++` paths: if they include `a/`/`b/` prefixes, it's `-p1`; if they reference files directly from the tarball root, it's `-p0`.

### 2. Initialise the cumulative scratch tree

Once per dep. The tree is a tarball extraction wrapped in a throwaway git repo just to get `git apply --3way`'s merge machinery — there's no real upstream history here.

```
cp -r /tmp/<dep-name>-new/<dep-name>-<new-version> /tmp/<dep-name>-patch-scratch/
cd /tmp/<dep-name>-patch-scratch/
git init -q
git add -A && git -c user.email=skill@local -c user.name=skill commit -q -m "base"
```

### 3. Walk `patches = [...]` in order

For each patch:

```
cd /tmp/<dep-name>-patch-scratch/
git apply --3way --whitespace=nowarn -p<N> <abs path to patch file>
```

Inspect:

- **Exit code 0, no `<<<<<<<` markers** (`git diff --check`) → clean apply (possibly with fuzz). Commit so subsequent patches see this patch's effects:
  ```
  git -c user.email=skill@local -c user.name=skill commit -aq --allow-empty -m "applied <patch-basename>"
  ```
  `--allow-empty` covers no-op patches (cases where upstream already merged the fix). Continue to the next patch.
- **Nonzero exit or conflict markers present** → failed. Document it (step 4), reset to the last successful commit (`git reset --hard HEAD && git clean -fd`), continue walking the list.

### 4. Document each failure

Per failed patch:

- **Leave the patch file on disk unchanged.**
- Capture the failure to `/tmp/<dep-name>-patch-<patch-basename>-conflict.txt`:
  - Full `git apply --3way` stderr + stdout.
  - For each conflicted file, the section with `<<<<<<< / ======= / >>>>>>>` markers.
- Surface a single block per failed patch to the user:

```
⚠ Patch did not apply cleanly: <patch_path>
  Conflict captured at /tmp/<dep-name>-patch-<patch-basename>-conflict.txt
  Likely cause: <one-line guess if obvious, otherwise "upstream rewrote target file">
  Manual action required before merge: resolve and refresh the patch, or drop it if upstream
    now provides the same fix.
```

### 5. After the walk — halt if any patch failed

If any patch in the list failed:

- The bump cannot proceed. Stop the skill here; do **not** start Step 9 (Verify) or Step 10 (Done).
- Print a consolidated summary of every failed patch (one per the block above) so the user has the full picture in a single message.
- The user resolves the failures (refreshes the patch, drops it from the list, or accepts that upstream merged it) and re-runs the skill.

If all patches applied cleanly, continue to Step 9.

---

## Edge cases

- **Patch applies to a file that no longer exists upstream.** Document as a failed apply with the cause noted. Often the right human follow-up is to delete the patch.
- **Patch file exists on disk but isn't in `patches = [...]`.** Surface to the user — the file is stale and should probably be deleted.

---

## What this step does NOT do

- **Rewrite patch files.** Detecting drift, not auto-resolving it. Refreshing a patch's hunks/line numbers is a human decision once the conflict is understood.
- **Resolve conflicts.**
- **Stop on the first patch failure.** Walk the whole list so the user sees every problem in one run.
- **Let the bump proceed past patch failures.** Across-step, any failure halts the skill.
- **Flip `patch_strip` / `patch_args` on MODULE.bazel.** The strip level convention belongs to the dep's existing decision; preserve it.
