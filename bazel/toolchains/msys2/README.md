# bash_shim

`bash_shim.exe` is Bazel's `--shell_executable` on Windows. It exists because
`cmd.exe` re-tokenizes arguments when running `.bat` files and truncates
`bash -c "..."` payloads at the first newline, which silently breaks
multi-line `ctx.actions.run_shell` actions.

The shim:
1. Reads its raw command line via `GetCommandLineW`, so embedded newlines
   survive intact.
2. Prepends hermetic `@msys2_base` (and optionally `@winlibs_mingw64`) bins
   to `PATH`.
3. Spawns `bash.exe` from `@msys2_base` with the original arguments via
   `CreateProcessW` and forwards its exit code.

## Rebuilding

Build via Bazel:

```powershell
bazel build //bazel/toolchains/msys2:bash_shim
```

On Windows, the built artifact is:

`bazel-bin/bazel/toolchains/msys2/bash_shim.exe`

If you need to refresh the committed launcher used by `.bazelrc`, copy that
artifact to:

`bazel/toolchains/msys2/bash_shim.exe`

Equivalent direct compile command (fallback):

```powershell
$gcc = "external/+winlibs_mingw_repository+winlibs_mingw64/bin/gcc.exe"
& $gcc -O2 -s -static -municode -o bazel/toolchains/msys2/bash_shim.exe `
    bazel/toolchains/msys2/bash_shim.c
```
