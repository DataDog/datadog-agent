# MSYS2 on Windows

Bazel on Windows needs MSYS2 bash for `genrule`, `run_shell`, and `rules_foreign_cc`
configure/make actions. The repo expects the tree at `C:/tools/msys64` (see
`.bazelrc`: `BAZEL_SH`, `--shell_executable`).

`@msys2_base` downloads a pinned MSYS2 base archive plus overlay pacman packages
(autotools stack). On Windows it installs under `MSYS2_INSTALL_ROOT` when bash is
missing or when a force install is requested.

## Current policy (relaxed)

- If `C:/tools/msys64/usr/bin/bash.exe` already exists, keep using it (no download,
  no reinstall).
- If bash is missing, `tools/bazel.bat` runs `bazel fetch @msys2_base//:bash_files`
  which downloads and installs the pinned tree.
- To replace the install entirely: `DD_BAZEL_MSYS2_FORCE_INSTALL=1` then `bazel build //...`
  (wrapper runs `bazel fetch --force @msys2_base//:bash_files --repo_env=MSYS2_FORCE_INSTALL=1`).
  Or manually:

```powershell
bazel fetch --force @msys2_base//:bash_files --repo_env=MSYS2_FORCE_INSTALL=1
```

If install still fails, check write access to `C:\tools` (admin / corporate policy).

## TODO

- Managed sentinel + auto-reinstall when the MODULE.bazel pin changes.
- Windows `rules_foreign_cc` toolchains backed by `@msys2_base` filegroups so
  make/autoconf/m4/etc. are action inputs (remote-cache correctness).
