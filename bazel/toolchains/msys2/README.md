# MSYS2 on Windows

Bazel on Windows needs MSYS2 bash for `genrule`, `run_shell`, and `rules_foreign_cc`
configure/make actions. The repo expects the tree at `C:/tools/msys64` (see
`.bazelrc`: `BAZEL_SH`, `--shell_executable`).

`@msys2_base` is a `local` repository rule that:

1. Downloads a pinned MSYS2 base archive plus overlay pacman packages (autotools stack)
2. On **native Windows Bazel** (`ctx.os.name` starts with `windows`), copies the tree to
   `MSYS2_INSTALL_ROOT` (default `C:/tools/msys64`) via `install.ps1`

Run Bazel from **cmd.exe or PowerShell**, not Git Bash or WSL — those report a
non-Windows OS to repository rules, so the host install step is skipped even though
`bazel fetch` succeeds.

## Current policy (relaxed)

- If `C:/tools/msys64/usr/bin/bash.exe` already exists, keep using it (no fetch).
- If bash is missing, `tools/bazel.bat` runs `bazel fetch --force @msys2_base//:bash_files`
  which triggers the repository rule install.
- Force reinstall: `DD_BAZEL_MSYS2_FORCE_INSTALL=1` then `bazel build //...`, or:

```powershell
bazel fetch --force @msys2_base//:bash_files --repo_env=MSYS2_FORCE_INSTALL=1
```

If install fails, the repository rule fails the fetch with stdout/stderr from
`install.ps1` (permissions, robocopy, missing archive, etc.).

## TODO

- Managed sentinel + auto-reinstall when the MODULE.bazel pin changes.
- Windows `rules_foreign_cc` toolchains backed by `@msys2_base` filegroups so
  make/autoconf/m4/etc. are action inputs (remote-cache correctness).
