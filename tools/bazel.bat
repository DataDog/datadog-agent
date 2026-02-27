@echo off
setlocal EnableDelayedExpansion
>nul chcp 65001

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if defined BAZEL_REAL if "%BAZELISK_SKIP_WRAPPER%"=="true" goto :bazelisk_ok
>&2 type "%~dp0bazelisk.md"
exit /b 2
:bazelisk_ok

:: Ensure `XDG_CACHE_HOME` denotes a directory
if not exist "%XDG_CACHE_HOME%" (
  if defined CI (
    >&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote a directory in CI!
    exit /b 2
  )
  if not defined DOTNET_RUNNING_IN_CONTAINER >nul 2>&1 sc query CExecSvc && set DOTNET_RUNNING_IN_CONTAINER=1
  if defined DOTNET_RUNNING_IN_CONTAINER (
    >&2 echo 💡 To persist caches across restarts, please set XDG_CACHE_HOME pointing to a mounted directory, e.g.:
    >&2 echo     docker.exe run --env=XDG_CACHE_HOME=C:\cache --volume="$HOME\.cache:C:\cache" ...
  )
)

:: Ensure `bazel` & managed toolchains honor `XDG_CACHE_HOME` if set: https://github.com/bazelbuild/bazel/issues/27808
if defined XDG_CACHE_HOME (
  set "XDG_CACHE_HOME=!XDG_CACHE_HOME:/=\!"
  if "!XDG_CACHE_HOME:~1,2!" neq ":\" if "!XDG_CACHE_HOME:~0,2!" neq "\\" (
    >&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote an absolute path!
    exit /b 2
  )
  :: https://pkg.go.dev/os#UserCacheDir
  set "GOCACHE=%XDG_CACHE_HOME%\go-build"
  :: https://wiki.archlinux.org/title/XDG_Base_Directory#Partial
  set "GOMODCACHE=%XDG_CACHE_HOME%\go\mod"
  :: https://pip.pypa.io/en/stable/topics/caching/#default-paths
  set "PIP_CACHE_DIR=%XDG_CACHE_HOME%\pip"
  set "bazel_home=%XDG_CACHE_HOME%\bazel"
  set bazel_home_startup_option="--output_user_root=!bazel_home!"
) else (
  set "XDG_CACHE_HOME=%~dp0..\.cache"
)

:: Check legacy max path length of 260 characters got lifted, or fail with instructions
set "more_than_260_chars=!XDG_CACHE_HOME!\more-than-260-chars"
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
if not defined CI (
  "%BAZEL_REAL%" !bazel_home_startup_option! %*
  exit /b !errorlevel!
)

:: Pass CI-specific options through `.user.bazelrc` so any nested `bazel run` and next `bazel shutdown` also honor them
(
  echo startup --connect_timeout_secs=5  # instead of 30s, for quicker iterations in diagnostics
  echo startup --local_startup_timeout_secs=30  # instead of 120s, to fail faster for diagnostics
  echo startup !bazel_home_startup_option:\=/!  # forward slashes: https://github.com/bazelbuild/bazel/issues/3275
  echo common --config=ci
) >"%~dp0..\user.bazelrc"

:: Diagnostics: print any stalled client/server before `bazel` execution
>&2 powershell -NoProfile -Command "Get-Process bazel,java -ErrorAction SilentlyContinue | Select-Object 🟡,ProcessName,StartTime"

:: Payload: execute `bazel` and remember exit status
"%BAZEL_REAL%" %*
set bazel_exit=!errorlevel!

:: Diagnostics: dump logs on non-trivial failures (https://bazel.build/run/scripts#exit-codes)
:: TODO(regis): adjust (probably `== 37`) next time a `cannot connect to Bazel server` error happens (#incident-42947)
set should_diagnose=1
for %%c in (0 1 3 34 36 48) do if !bazel_exit!==%%c set should_diagnose=0
if !should_diagnose!==1 (
  >&2 echo 🔴 Bazel failed [!bazel_exit!], dumping available info in !bazel_home! ^(excluding junctions^):
  for /f "delims=" %%d in ('dir /a:d-l /b "!bazel_home!"') do (
    >&2 echo 🟡 [%%d]
    for %%f in ("!bazel_home!\%%d\java.log.*" "!bazel_home!\%%d\server\*") do (
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

:: Stop `bazel` (if still running) to close files and proceed with cleanup
>&2 "%BAZEL_REAL%" shutdown --ui_event_filters=-info
>&2 del /f /q "%~dp0..\user.bazelrc"

:: Done
exit /b !bazel_exit!
