# bash_shim

`bash_shim.exe` is Bazel's `--shell_executable` on Windows. It exists because
`cmd.exe` re-tokenises arguments when running `.bat` files and truncates
`bash -c "..."` payloads at the first newline, which silently breaks
multi-line `ctx.actions.run_shell` actions (e.g. `GoMockSourceGen`).

The shim:
1. Reads its raw command line via `GetCommandLineW`, so embedded newlines
   survive intact.
2. Prepends the hermetic `@msys2_base` and `@winlibs_mingw64` bin directories
   to `PATH`.
3. Spawns `bash.exe` from `@msys2_base` with the original arguments via
   `CreateProcessW` and forwards its exit code.

## Rebuilding

The `.exe` is committed so first-time builds work without bootstrapping a
toolchain. Rebuild only when `bash_shim.c` changes:

```powershell
$gcc = "external/+winlibs_mingw_repository+winlibs_mingw64/bin/gcc.exe"
& $gcc -O2 -s -static -municode -o bazel/toolchains/msys2/bash_shim.exe `
    bazel/toolchains/msys2/bash_shim.c
git add bazel/toolchains/msys2/bash_shim.exe
```

Resolve `$gcc` relative to your Bazel output base if `external/` is not
symlinked into the workspace; `bazel info output_base` prints the location.
