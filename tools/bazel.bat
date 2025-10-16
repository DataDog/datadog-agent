@echo off & setlocal EnableDelayedExpansion & if not defined bb (set "bb=%TEMP%\%~n0-%RANDOM%.bat" & more /e /p "%~f0" >"!bb!" & call "!bb!" %* & set "rc=!errorlevel!" & del /q "!bb!" & exit /b !rc!)
:: Above one-liner copies the present script to a temporary file with normalized Windows line endings and executes it.
:: Works regardless of actual source line endings because the pseudo-shebang is interpreted before the first newline.

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if not defined BAZEL_REAL (
  >&2 type "%~dp0bazelisk.md"
  exit /b 1
) else if not "%BAZELISK_SKIP_WRAPPER%"=="true" (
  >&2 type "%~dp0bazelisk.md"
  exit /b 2
)

:: Not in CI: simply execute `bazel` - done
if not defined CI_PROJECT_DIR (
  "%BAZEL_REAL%" %*
  exit /b !errorlevel!
)

:: In CI: first, verify directory environment variables are set and normalize their paths
for %%v in (BAZEL_DISK_CACHE BAZEL_OUTPUT_USER_ROOT BAZEL_REPO_CONTENTS_CACHE RUNNER_TEMP_PROJECT_DIR VSTUDIO_ROOT) do (
  if not defined %%v (
    >&2 echo %~nx0: %%v: unbound variable
    exit /b 3
  )
  :: Path separators: `bazel` is fine with both `/` and `\\` but fails with `\`, so the simplest is to favor `/`
  set "%%v=!%%v:\=/!"
)
:: TODO(regis, if later needed): set "BAZEL_SH=C:/tools/msys64/usr/bin/bash.exe"
set "BAZEL_VS=!VSTUDIO_ROOT!"
set "ext_repo_contents_cache=!RUNNER_TEMP_PROJECT_DIR!/bazel-repo-contents-cache"

:: Externalize `--repo_contents_cache` to the job's sibling temporary directory created alongside $CI_PROJECT_DIR
:: - https://github.com/bazelbuild/bazel/issues/26384 for why
:: - https://docs.gitlab.com/runner/configuration/advanced-configuration/ for `RUNNER_TEMP_PROJECT_DIR`
:: - https://sissource.ethz.ch/sispub/gitlab-ci-euler-image/-/blob/main/entrypoint.sh#L43 for inspiration
if exist "!BAZEL_REPO_CONTENTS_CACHE!" (
  call :robomove "!BAZEL_REPO_CONTENTS_CACHE!" "!ext_repo_contents_cache!"
  if !errorlevel! neq 0 exit /b !errorlevel!
)

:: Pass CI-specific options through `.user.bazelrc` so any nested `bazel run` and next `bazel shutdown` also honor them
(
  echo startup --connect_timeout_secs=30
  echo startup --output_user_root=!BAZEL_OUTPUT_USER_ROOT!
  echo common --config=cache
  echo common --repo_contents_cache=!ext_repo_contents_cache!
  echo build --disk_cache=!BAZEL_DISK_CACHE!
) >"%CI_PROJECT_DIR%\user.bazelrc"

:: Diagnostics: print any stalled client/server before `bazel` execution
>&2 powershell -NoProfile -Command "Get-Process bazel,java -ErrorAction SilentlyContinue | Select-Object 🟡,ProcessName,StartTime"

:: Payload: execute `bazel` and remember exit status
"%BAZEL_REAL%" %*
set "bazel_exit=!errorlevel!"

:: Diagnostics: dump JVM output on failure
if !bazel_exit! neq 0 (
  >&2 echo 🟡 JVM output:
  >&2 type "!BAZEL_OUTPUT_USER_ROOT!\server\jvm.out"
  exit /b !bazel_exit!
)

:: Stop `bazel` (if still running) to close files and proceed with cleanup
>&2 "%BAZEL_REAL%" shutdown --ui_event_filters=-info
>&2 del /f /q "%CI_PROJECT_DIR%\user.bazelrc"

:: Reintegrate `--repo_contents_cache` to original directory
if exist "!ext_repo_contents_cache!" (
  call :robomove "!ext_repo_contents_cache!" "!BAZEL_REPO_CONTENTS_CACHE!"
  if !errorlevel! neq 0 exit /b !errorlevel!
)

:: Done
exit /b 0

:robomove
:: Contrarily to `copy`, `move` and `xcopy`, `robocopy` avoids messing up with recursive symlinks, thanks to `/xj`
>&2 robocopy "%~1" "%~2" /b /copyall /dcopy:dat /mir /move /ndl /nfl /njh /njs /np /secfix /sl /timfix /w:0 /xj
:: See: https://ss64.com/nt/robocopy-exit.html
set /a rc=!errorlevel! ^& (8 ^| 16)
if exist "%~1" (
  >&2 echo 🟡 Purging leftovers, most likely due to recursive symbolic links/junction points:
  >&2 dir /a /b /s "%~1"
  >&2 rmdir /q /s "%~1"
)
exit /b !rc!
