@echo off
:: Shell shim invoked via --shell_executable on Windows so that ctx.actions.run_shell
:: and friends (genrule, rules_foreign_cc, ...) reach the hermetic MSYS2 bash
:: provided by //bazel/toolchains/msys2:msys2.bzl instead of whatever bash.exe
:: happens to be on the action's PATH (typically C:\Windows\System32\bash.exe,
:: which is the WSL stub).
::
:: Bazel resolves --shell_executable relative to the workspace root at startup,
:: then invokes us with the action's execroot (output_base\execroot\_main) as
:: CWD. External repos are only staged under execroot when declared as action
:: inputs; the make_tool, configure_make and similar rules_foreign_cc actions
:: don't declare @msys2_base, so we reach the canonical extraction path under
:: output_base by going two levels up from CWD.
::
:: Reaching outside execroot is safe under --strategy=standalone (which the
:: workspace .bazelrc pins on Windows) but would fail under sandbox / RBE.
::
:: Required because bazelbuild/bazel#21089 — ctx.actions.run_shell does not
:: consult sh_toolchain — is still open upstream. Once that lands the shim and
:: this .bazelrc flag can be dropped in favour of the toolchain registration
:: alone.
setlocal
set "MSYS2_ROOT=%cd%\..\..\external\+msys2_base_repository+msys2_base"
set "MINGW_ROOT=%cd%\..\..\external\+winlibs_mingw_repository+winlibs_mingw64"
set "BASH=%MSYS2_ROOT%\usr\bin\bash.exe"
if not exist "%BASH%" (
    echo bash_shim: hermetic bash not found at %BASH% 1>&2
    echo bash_shim: run 'bazelisk fetch @msys2_base//...' to materialise it 1>&2
    exit /b 1
)
if not exist "%MINGW_ROOT%\bin\gcc.exe" (
    echo bash_shim: hermetic gcc not found at %MINGW_ROOT%\bin\gcc.exe 1>&2
    echo bash_shim: run 'bazelisk fetch @winlibs_mingw64//...' to materialise it 1>&2
    exit /b 1
)
:: Prepend MSYS2's usr/bin (sed, gawk, grep, tar, ...) and WinLibs' bin/ (gcc,
:: g++, ld, ...) so bash and any tools it execs resolve to the hermetic copies
:: rather than whatever leaked in via the action's PATH. MSYS2 first so its
:: msys-2.0.dll-linked coreutils win over any same-name native Windows tools
:: that may ship in WinLibs.
set "PATH=%MSYS2_ROOT%\usr\bin;%MINGW_ROOT%\bin;%PATH%"
"%BASH%" %*
exit /b %ERRORLEVEL%
