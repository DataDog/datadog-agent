# Hermetic MSYS2

`@msys2_base` is Bazel's hermetic MSYS2 distribution on Windows. It provides
the `bash.exe` used by `ctx.actions.run_shell`, `genrule`, and shell-backed
rules.

On Windows, `tools/bazel.bat` materializes `@msys2_base//:bash_files`, computes
the absolute path to the extracted `usr/bin/bash.exe` in the active Bazel
`output_base`, and passes it to Bazel via `--shell_executable`.

This keeps the shell path local to each environment (host, container, CI
worker) while letting Bazel's repository cache reuse the pinned MSYS2 archive
across worktrees and workspaces.

## Manual Fetch

The wrapper fetches MSYS2 automatically. To materialize it explicitly:

```powershell
bazel fetch @msys2_base//:bash_files
```
