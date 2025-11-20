@echo off
setlocal EnableDelayedExpansion
>nul chcp 65001

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if defined BAZEL_REAL if "%BAZELISK_SKIP_WRAPPER%"=="true" goto :bazelisk_ok
>&2 type "%~dp0bazelisk.md"
exit /b 2
:bazelisk_ok

:: Check legacy max path length of 260 characters got lifted, or fail with instructions
if defined XDG_CACHE_HOME (set "cache_home=%XDG_CACHE_HOME%") else (set "cache_home=%~dp0..\.cache")
set "more_than_260_chars=!cache_home!\more-than-260-chars"
for /l %%i in (1,1,26) do set "more_than_260_chars=!more_than_260_chars!\123456789"
if not exist "!more_than_260_chars!" (
  2>nul mkdir "!more_than_260_chars!"
  if !errorlevel! neq 0 (
    >&2 echo 🔴 For `bazel` to work properly, please lift the 260-character path limit from your Windows OS as per:
    >&2 echo - either https://learn.microsoft.com/en-us/windows/win32/fileio/maximum-file-path-limitation
    >&2 echo - or https://andrewlock.net/fixing-max_path-issues-in-gitlab/#window-s-maximum-path-length-limitation-
    exit /b 2
  )
)

:: Not in CI: simply execute `bazel` - done
if not defined CI_PROJECT_DIR (
  "%BAZEL_REAL%" %*
  exit /b !errorlevel!
)

:: In CI: first, verify directory environment variables are set and normalize their paths
for %%v in (BAZEL_DISK_CACHE BAZEL_OUTPUT_USER_ROOT BAZEL_REPO_CONTENTS_CACHE VSTUDIO_ROOT) do (
  if not defined %%v (
    >&2 echo %~nx0: %%v: unbound variable
    exit /b 2
  )
  :: Path separators: `bazel` is fine with both `/` and `\\` but fails with `\`, so the simplest is to favor `/`
  set "%%v=!%%v:\=/!"
)
set "BAZEL_VS=!VSTUDIO_ROOT!"

:: Pass CI-specific options through `.user.bazelrc` so any nested `bazel run` and next `bazel shutdown` also honor them
(
  echo startup --connect_timeout_secs=5  # instead of 30s, for quicker iterations in diagnostics
  echo startup --local_startup_timeout_secs=30  # instead of 120s, to fail faster for diagnostics
  echo startup --output_user_root=!BAZEL_OUTPUT_USER_ROOT!
  echo common --config=ci
  echo common --disk_cache=!BAZEL_DISK_CACHE!
  echo common --repo_contents_cache=!BAZEL_REPO_CONTENTS_CACHE!
) >"%~dp0..\user.bazelrc"

:: Diagnostics: print any stalled client/server before `bazel` execution
>&2 powershell -NoProfile -Command "Get-Process bazel,java -ErrorAction SilentlyContinue | Select-Object 🟡,ProcessName,StartTime"

:: Payload: execute `bazel` and remember exit status
"%BAZEL_REAL%" %*
set "bazel_exit=!errorlevel!"

:: Diagnostics: dump logs on non-trivial failures (https://bazel.build/run/scripts#exit-codes)
:: TODO(regis): adjust (probably `== 37`) next time a `cannot connect to Bazel server` error happens (#incident-42947)
if !bazel_exit! geq 2 (
  >&2 echo 🟡 Bazel failed [!bazel_exit!], dumping available info in !BAZEL_OUTPUT_USER_ROOT! ^(excluding junctions^):
  for /f "delims=" %%d in ('dir /a:d-l /b "!BAZEL_OUTPUT_USER_ROOT!"') do (
    >&2 echo 🟡 [%%d]
    for %%f in ("!BAZEL_OUTPUT_USER_ROOT!\%%d\java.log.*" "!BAZEL_OUTPUT_USER_ROOT!\%%d\server\*") do (
      if exist "%%f" (
        >&2 echo 🟡 %%f:
        >&2 type "%%f"
        >&2 echo.
      ) else (
        >&2 echo 🟡 %%f doesn't exist
      )
    )
  )
)
if !bazel_exit! neq 0 exit /b !bazel_exit!

:: Stop `bazel` (if still running) to close files and proceed with cleanup
>&2 "%BAZEL_REAL%" shutdown --ui_event_filters=-info
>&2 del /f /q "%~dp0..\user.bazelrc"

:: Done
exit /b 0
