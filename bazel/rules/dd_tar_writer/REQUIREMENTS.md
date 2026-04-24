# Requirements: dd_tar_writer

## Purpose

`dd_tar_writer` is a Bazel-compatible tar archive builder used in the Datadog Agent
package build pipeline. It is a drop-in replacement for the `build_tar.py` tool
shipped with `rules_pkg`, with one additional capability: it produces a **sidecar
MD5 manifest file** listing the checksum of every regular file written into the
archive.

This sidecar file is consumed by the `dd_tar` Bazel rule (see
`bazel/rules/dd_tar.bzl`) which exposes it via `OutputGroupInfo`, making it
available to downstream rules such as `pkg_deb`.

---

## Inputs

### Positional

None. All inputs are provided via flags.

### Flags

The following flags match `rules_pkg`'s `build_tar.py` exactly and must remain
compatible with however `rules_pkg` invokes the tool:

| Flag | Type | Description |
|------|------|-------------|
| `--output` | string (required) | Path to the output tar file |
| `--manifest` | string | Path to the rules_pkg JSON manifest file |
| `--mode` | octal string | Default file mode applied to all files (e.g. `0755`) |
| `--mtime` | int or `"portable"` | Default mtime. `"portable"` = 946684800 (2000-01-01 UTC) |
| `--tar` | string (repeatable) | Existing tar file to merge into the output |
| `--deb` | string (repeatable) | Debian package whose `data.tar.*` to merge |
| `--directory` | string | Prefix prepended to all archive paths |
| `--compression` | `gz`, `bz2`, `xz` | Compression algorithm |
| `--compressor` | string | External compressor command, e.g. `pigz -p 4` |
| `--compression_level` | int | Compression level (0–9, or -1 for default) |
| `--modes` | `path=octal` (repeatable) | Per-file mode override |
| `--owners` | `path=uid.gid` (repeatable) | Per-file numeric owner override |
| `--owner` | `uid.gid` | Default numeric owner (default: `0.0`) |
| `--owner_name` | `user.group` | Default owner name |
| `--owner_names` | `path=user.group` (repeatable) | Per-file owner name override |
| `--stamp_from` | string | Path to Bazel volatile status file; overrides `--mtime` with `BUILD_TIMESTAMP` |
| `--create_parents` | bool flag | Auto-create implied parent directories |
| `--allow_dups_from_deps` | bool flag | Suppress duplicate-path warnings |
| `--preserve_mode` | bool flag | Use the source file's actual permissions |
| `--preserve_mtime` | bool flag | Use the source file's actual mtime |

**New flag (dd_tar_writer extension, not in build_tar.py):**

| Flag | Type | Description |
|------|------|-------------|
| `--md5sums_output` | string | If set, write the MD5 sidecar to this path |

### Response files

Arguments may be placed in a file and passed as `@path/to/file`, one argument
per line. This matches Python argparse `fromfile_prefix_chars='@'` semantics and
is required for Bazel's param-file mechanism.

---

## Inputs from `--manifest`

The manifest is a JSON array of objects, each describing one entry to add to the
archive. The schema comes from `rules_pkg`'s `pkg/private/manifest.py`.

### Entry fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Entry type (see below) |
| `dest` | string | yes | Destination path in the archive |
| `src` | string | no | Source path on disk |
| `mode` | string | no | Octal mode string, overrides default |
| `user` | string | no | Owner username |
| `group` | string | no | Owner group name |
| `uid` | int | no | Numeric user ID |
| `gid` | int | no | Numeric group ID |
| `origin` | string | no | Informational only |
| `repository` | string | no | Informational only |

### Entry types

| Value | Meaning |
|-------|---------|
| `"file"` | Regular file: content taken from `src` |
| `"symlink"` | Symbolic link: `dest` → `src` (literal) |
| `"raw_symlink"` | Symbolic link: `dest` → target read from `os.Readlink(src)` |
| `"dir"` | Empty directory at `dest` |
| `"tree"` | Directory tree rooted at `src`, placed under `dest` |
| `"empty-file"` | Zero-byte regular file at `dest` |

### Attribute precedence (low → high)

1. Tool defaults (`--owner 0.0`, mode derived from file executable bit)
2. Tool-level flag overrides (`--mode`, `--owner`, `--owner_name`)
3. Per-file flag overrides (`--modes`, `--owners`, `--owner_names`)
4. Manifest entry fields (`mode`, `uid`/`gid`, `user`/`group`)

---

## Outputs

### Tar archive (`--output`)

- Format: **GNU tar** (`tar.FormatGNU` in Go / `tarfile.GNU_FORMAT` in Python)
- Entries are written in manifest order; merging tars come after manifest entries
- Parent directories are only auto-created if `--create_parents` is set
- Duplicate paths: first occurrence wins; subsequent occurrences are silently
  dropped (with a warning to stderr unless `--allow_dups_from_deps`)
- Symlinks and directories included as tar entries but carry no file content

### MD5 sidecar (`--md5sums_output`)

Written only when the flag is provided.

- One line per **regular file** (type `file`, `empty-file`, or regular file from
  a merged tar)
- Symlinks and directories are **excluded**
- Format identical to `md5sum(1)`:

  ```
  <32-hex-chars>  <archive-path>
  ```

  Note: two spaces between hash and path. Archive paths do not have a leading `/`.

- Lines are written in the order files were added to the archive (manifest order
  first, then merged tars, then merged debs)
- Empty files produce the MD5 of the empty string:
  `d41d8cd98f00b204e9800998ecf8427e`

---

## Invariants

1. **Determinism**: given identical inputs and flags, the output tar and MD5 sidecar
   are bit-for-bit identical. Callers must ensure `--mtime` or `--stamp_from` is
   set consistently. Gzip output sets `Header.ModTime` to the same value as the
   tar mtime to prevent the gzip header from being a source of non-determinism.

2. **MD5 coverage**: every regular file whose content is written into the tar is
   listed in the sidecar. The sidecar never lists a file that is not in the tar.

3. **No self-reference**: the MD5 sidecar is a separate file; it is not included
   in the tar it describes, so there is no circular dependency.

4. **GNU format**: the tar output uses GNU format for compatibility with
   `pkg_deb`'s `make_deb.py` and the broader Linux toolchain.

5. **CLI compatibility**: all flags listed above that also exist in `build_tar.py`
   must behave identically to `build_tar.py` so that the tool can be substituted
   by changing only the `_tar_tool` attribute of `pkg_tar`.

---

## When rules_pkg changes its API

The interface between `dd_tar.bzl` and this binary is derived from how
`rules_pkg`'s `tar.bzl` invokes `build_tar.py`. When upgrading rules_pkg, check:

1. **New flags added to `build_tar.py`**: add the corresponding flag to this tool.
   Check `pkg/private/tar/build_tar.py` in the new rules_pkg version.

2. **Manifest schema changes**: compare `pkg/private/manifest.py`. New fields in
   `ManifestEntry` or new entry types must be handled. Unknown types should produce
   a clear error message.

3. **Archive format changes**: if rules_pkg switches from GNU to PAX format, update
   the `Format` field in every `tar.Header` written by this tool.

4. **Output provider changes**: if `tar.bzl` changes what it returns (e.g., exposes
   a new output group), update `dd_tar.bzl` correspondingly. The MD5 group must
   remain in `OutputGroupInfo`.

5. **Invocation changes**: check whether `build_tar.py` is now called differently
   (param files, new arg order, new environment variables). Update this tool to
   match.

The requirements in this document should be re-read at upgrade time to verify the
implementation still satisfies them.
