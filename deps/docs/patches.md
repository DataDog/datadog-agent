# Patch refresh

Expands Step 8. Step 1 captured the `patches = [...]` list (filesystem paths) and the strip level (`patch_args` / `patch_strip`). Operate on that data, not on filesystem globs.

---

## Goal

For each patch in `patches = [...]`, in order, against the new tarball: confirm it still applies, or record that it doesn't. Detect drift; don't rewrite patch files.

**Two-level policy:**

- **Within this step:** keep walking the patch list even after a failure, so one run surfaces every failed patch.
- **Across the bump:** if any patch failed, the bump halts here. Do not proceed to Step 9 or Step 10. The user resolves the failed patches manually and re-runs.

---

## Procedure

Bazel applies `patches = [...]` cumulatively in order at fetch time — patch N sees the tree after patches 1..N-1. Mirror that with one cumulative scratch tree per dep.

### 1. Resolve the strip level

`patch_args = ["-pN", ...]` or `patch_strip = N` from Step 1 is authoritative. If neither is present, `http_archive`'s default is `-p0` (not `git apply`'s `-p1`). Verify against each patch's `---` / `+++` paths: `a/`/`b/` prefixes → `-p1`; files from the tarball root → `-p0`.

### 2. Initialise the cumulative scratch tree

Once per dep. A throwaway git repo wraps the extraction so each applied patch can be committed and a failed one reset.

```
cp -r /tmp/<dep-name>-new/<dep-name>-<new-version> /tmp/<dep-name>-patch-scratch/
cd /tmp/<dep-name>-patch-scratch/
git init -q
git add -A && git commit -q -m "base"
```

### 3. Walk `patches = [...]` in order

Apply each patch with the same tool Bazel uses, so the result matches the fetch:

- `patch_tool` set on the http_archive -> Bazel runs `<patch_tool> -p<N> < <patch>`:
  ```
  cd /tmp/<dep-name>-patch-scratch/
  <patch_tool> -p<N> --forward --batch --no-backup-if-mismatch < <abs path to patch file>
  ```
- `patch_tool` unset -> Bazel's built-in patcher, which accepts no fuzz. Closest equivalent:
  ```
  cd /tmp/<dep-name>-patch-scratch/
  git apply --whitespace=nowarn -p<N> <abs path to patch file>
  ```

Inspect:

- **Exit code 0, no `*.rej` files** -> clean apply. Report any fuzz or offset in the tool output as a warning (the patch may need a refresh). Commit so subsequent patches see this patch's effects:
  ```
  git add -A && git commit -q --allow-empty -m "applied <patch-basename>"
  ```
  `--allow-empty` covers no-op patches. Continue to the next patch.
- **Nonzero exit or `*.rej` files present** -> failed. Document it (step 4), reset (`git reset --hard HEAD && git clean -fd`), continue walking the list.

### 4. Document each failure

Per failed patch:

- **Leave the patch file on disk unchanged.**
- Capture the failure to `/tmp/<dep-name>-patch-<patch-basename>-conflict.txt`:
  - Full stderr + stdout of the apply command.
  - The contents of each `*.rej` file, if any.
- Surface a single block per failed patch:

```
⚠ Patch did not apply cleanly: <patch_path>
  Conflict captured at /tmp/<dep-name>-patch-<patch-basename>-conflict.txt
  Likely cause: <one-line guess if obvious, otherwise "upstream rewrote target file">
  Manual action required before merge: resolve and refresh the patch, or drop it if upstream
    now provides the same fix.
```

### 5. After the walk — halt if any patch failed

If any patch failed:

- Stop the skill here; do **not** start Step 9 or Step 10.
- Print a consolidated summary of every failed patch (one block each) in a single message.
- The user resolves the failures and re-runs the skill.

If all patches applied cleanly, continue to Step 9.

---

## Edge cases

- **Patch applies to a file that no longer exists upstream.** Document as a failed apply with the cause noted.
- **Patch file exists on disk but isn't in `patches = [...]`.** Surface to the user — stale, probably delete.

---

## What this step does NOT do

- Rewrite patch files.
- Resolve conflicts.
- Stop on the first patch failure.
- Let the bump proceed past patch failures.
- Flip `patch_strip` / `patch_args` on MODULE.bazel.
