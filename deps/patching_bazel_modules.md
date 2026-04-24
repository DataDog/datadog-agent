# Patching Bazel Modules

This document describes the technique used to apply small, surgical changes to
upstream Bazel modules (dependencies declared in `MODULE.bazel`) without
maintaining a full fork of the upstream repository.

## When to use this

Use this when you need to patch an upstream module and:
- The change is not yet accepted upstream or not yet released to the BCR, **and**
- The patch is small enough that the full modified file is reviewable in a PR.

## Directory layout

For each patched module named `<mod>`:

```
third_party/<mod>/
  <path/to/patched/file>   # full modified source, committed for reviewability
  patches/
    BUILD.bazel            # package delimiter (makes files referenceable as labels)
    <name>.patch           # generated unified diff against the pinned upstream commit
  generate_patches.sh      # regenerates the patch(es); update COMMIT here too
```

Example for `rules_pkg`:
```
third_party/rules_pkg/
  pkg/private/tar/tar.bzl
  patches/
    BUILD.bazel
    tar_bzl.patch
  generate_patches.sh
```

## How MODULE.bazel references the patch

Use `git_override` with a `patches` attribute pointing to the patch label:

```python
git_override(
    module_name = "rules_pkg",
    commit = "deadbeef...",          # exact upstream commit
    remote = "https://github.com/bazelbuild/rules_pkg.git",
    patch_strip = 1,                 # equivalent to patch -p1; strips a/ and b/ prefixes
    patches = [
        "//third_party/rules_pkg/patches:tar_bzl.patch",
    ],
)
```

`patch_strip = 1` strips the first path component from the diff headers.  The
patch is generated with `--label "a/<upstream-path>"` and
`--label "b/<upstream-path>"`, so `a/pkg/private/tar/tar.bzl` is stripped to
`pkg/private/tar/tar.bzl` — the path within the rules_pkg repository.

## Generating / updating a patch

Run the script from the repository root:

```bash
bash third_party/rules_pkg/generate_patches.sh
```

The script downloads the upstream file at the pinned commit, diffs it against
the committed modified file, and writes the result to `patches/`.  Commit both
the regenerated `.patch` file and any changes to the modified source file.

## Updating the upstream commit

1. Update the `commit` in `MODULE.bazel`'s `git_override`.
2. Update `COMMIT` in `generate_patches.sh` to the same value.
3. Update the modified source file(s) in `third_party/<mod>/` to incorporate
   upstream changes while preserving our modifications.
4. Run `bash third_party/<mod>/generate_patches.sh` to regenerate patches.
5. Commit everything together (updated source, patch, MODULE.bazel, script).

## Why both the patch and the full file are committed

| Artifact | Purpose |
|----------|---------|
| Full modified file | Code review: reviewers see complete context without mentally applying diffs |
| Patch file | Bazel build: applied at module fetch time; precise specification of changes |
| Generate script | Keeps them in sync; documents the pinned upstream commit |

## Cross-module label references in patched .bzl files

If a patched `.bzl` file needs to reference a target in the **main repository**
(not the patched module), use the canonical `@@//` label prefix:

```python
"_some_tool": attr.label(
    default = Label("@@//bazel/path/to:tool"),
    ...
),
```

`@@` is the canonical label prefix for the root module in Bazel bzlmod.
Regular `//path:target` labels inside a patched module's `.bzl` files resolve
to that module's repository, not the main repo.

## Future: switching to archive_override

When ready to pin to a release archive instead of a git commit, replace
`git_override` with `archive_override` and keep the same `patches` attribute:

```python
archive_override(
    module_name = "rules_pkg",
    urls = ["https://github.com/bazelbuild/rules_pkg/releases/download/v1.x.y/rules_pkg-1.x.y.tar.gz"],
    integrity = "sha256-...",
    strip_prefix = "rules_pkg-1.x.y",
    patch_strip = 1,
    patches = [
        "//third_party/rules_pkg/patches:tar_bzl.patch",
    ],
)
```
