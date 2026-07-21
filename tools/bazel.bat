@echo off
setlocal EnableDelayedExpansion
>nul chcp 65001

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if defined BAZEL_REAL if "%BAZELISK_SKIP_WRAPPER%"=="true" goto :bazelisk_ok
>&2 type "%~dp0bazelisk.md"
exit /b 2
:bazelisk_ok

:: Ensure `XDG_CACHE_HOME` denotes a directory
if not defined DOTNET_RUNNING_IN_CONTAINER >nul 2>&1 sc query CExecSvc && set DOTNET_RUNNING_IN_CONTAINER=1
if not exist "%XDG_CACHE_HOME%" (
  if defined CI (
    >&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote a directory in CI!
    exit /b 2
  )
  if defined DOTNET_RUNNING_IN_CONTAINER (
    >&2 echo 💡 To persist caches across restarts, please set XDG_CACHE_HOME pointing to a mounted directory, e.g.:
    >&2 echo     docker.exe run --env=XDG_CACHE_HOME=C:\cache --volume="$HOME\.cache:C:\cache" ...
  )
)

:: Ensure `bazel` & managed toolchains honor `XDG_CACHE_HOME`
set "extra_args="
if defined XDG_CACHE_HOME (
  set "XDG_CACHE_HOME=!XDG_CACHE_HOME:/=\!"
  if "!XDG_CACHE_HOME:~1,2!" neq ":\" if "!XDG_CACHE_HOME:~0,2!" neq "\\" (
    >&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote an absolute path!
    exit /b 2
  )
  set "GOCACHE=!XDG_CACHE_HOME!\go-build"
  set "GOMODCACHE=!XDG_CACHE_HOME!\go\mod"
  set "PIP_CACHE_DIR=!XDG_CACHE_HOME!\pip"
  :: https://github.com/bazelbuild/bazel/issues/27808
  set "bazel_home=!XDG_CACHE_HOME!\bazel"
  set startup_options="--output_user_root=!bazel_home!"
  :: Use container-scoped `outputBase` to prevent races on `outputUserRoot\<same workspace hash>\server\jvm.out`
  if defined DOTNET_RUNNING_IN_CONTAINER set startup_options=!startup_options! "--output_base=%SYSTEMDRIVE%\bob"
  set extra_args="--disk_cache=!bazel_home!\disk-cache"
  :: https://github.com/bazelbuild/bazel/issues/26384
  for %%i in ("%~dp0..\.cache") do if "!XDG_CACHE_HOME!" == "%%~fi" set "extra_args=!extra_args! --repo_contents_cache="
  if defined CI if not defined GITHUB_ACTIONS set "extra_args=!extra_args! --config=ci --config=cache:frontend"
) else (
  :: Without XDG_CACHE_HOME, fall back Go caches to official defaults so Go repo rules work under strict repo_env
  if not defined GOCACHE set "GOCACHE=%LOCALAPPDATA%\go-build"
  if not defined GOMODCACHE (
    if defined GOPATH (for /f "tokens=1 delims=;" %%i in ("%GOPATH%") do set "gp=%%i") else set "gp=%USERPROFILE%\go"
    set "GOMODCACHE=!gp!\pkg\mod"
    set "gp="
  )
)

:: Local developer remote cache selection (CI selects its own endpoint above).
if not defined CI call :remote_cache_select

:: Check legacy max path length of 260 characters got lifted, or fail with instructions
for %%i in ("%~dp0..\.cache") do if defined XDG_CACHE_HOME (set "more_than_260_chars=!XDG_CACHE_HOME!") else set "more_than_260_chars=%%~fi"
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

:: Check 8.3 short names are enabled, or fail with instructions
:: TODO(agent-build): remove once https://github.com/bazelbuild/bazel/pull/29921 (or equivalent) is in effect
set "more_than_8dot3_chars=%TEMP%\123456789.1234"
2>nul del /f /q "!more_than_8dot3_chars!"
>"!more_than_8dot3_chars!" type nul
for %%i in ("!more_than_8dot3_chars!") do if "%%~nxi"=="%%~snxi" (
  >&2 echo 🔴 For `bazel` to work properly, please enable 8.3 short names on %%~di:
  >&2 echo     fsutil 8dot3name set %%~di 0
  exit /b 2
)

set "args=%*"
if defined args if defined extra_args call :insert_extra_args
"%BAZEL_REAL%" !startup_options! !args!
exit /b !errorlevel!

:: "--startup cmd ..." -> "--startup cmd --config=ci ..."
:insert_extra_args
set "startup_args="
set "next_args=!args!"
:parse_next_arg
set "cmd="
for /f "tokens=1* delims= " %%i in ("!next_args!") do (
  set "arg=%%~i"
  if "!arg:~0,1!" equ "-" (
    set "startup_args=!startup_args! %%i"
    set "next_args=%%j"
  ) else (
    if defined startup_args set "startup_args=!startup_args:~1! "
    set "cmd=%%i"
    set "args=!startup_args!!cmd! !extra_args! %%j"
  )
)
if not defined cmd if defined next_args goto :parse_next_arg
exit /b

:: Buildbarn remote cache auto-selection. Policy via DD_BAZEL_REMOTE_CACHE:
:: auto (default) | on | off. Appends --config=cache to extra_args when enabled.
:remote_cache_select
:: An explicit cache config on the command line always wins.
echo %* | findstr /C:"--config=cache" /C:"--config=no-remote-cache" >nul && goto :eof
if not defined DD_BAZEL_REMOTE_CACHE set "DD_BAZEL_REMOTE_CACHE=auto"
if /i "%DD_BAZEL_REMOTE_CACHE%"=="off" goto :eof
if /i "%DD_BAZEL_REMOTE_CACHE%"=="on" (
  if defined extra_args (set "extra_args=!extra_args! --config=cache") else set "extra_args=--config=cache"
  goto :eof
)
if /i not "%DD_BAZEL_REMOTE_CACHE%"=="auto" (
  >&2 echo 🔴 Unknown DD_BAZEL_REMOTE_CACHE=%DD_BAZEL_REMOTE_CACHE%, expected auto^|on^|off
  goto :eof
)
call :remote_cache_eligible || goto :eof
if defined extra_args (set "extra_args=!extra_args! --config=cache") else set "extra_args=--config=cache"
goto :eof

:remote_cache_eligible
set "_have_token="
if defined BUILDBARN_ID_TOKEN set "_have_token=1"
if defined DOTNET_RUNNING_IN_CONTAINER (
  if not defined _have_token (
    >&2 echo 💡 Bazel remote cache skipped: no Buildbarn token in this container. Mint one on the host and inject it, e.g.:
    >&2 echo     docker.exe run --env=BUILDBARN_ID_TOKEN=^<token^> ...
    exit /b 1
  )
) else (
  if not defined _have_token where vault >nul 2>&1 || exit /b 1
)
call :remote_cache_reachable
exit /b !errorlevel!

:: Reachability probe. Any HTTPS response (incl. gRPC's 415) counts as
:: reachable; only a connection/TLS failure counts as unreachable. A positive
:: result is cached under %TEMP% (sticky until reboot clears it); a negative
:: result is not cached, so a VPN reconnect is picked up on the next build.
:remote_cache_reachable
set "_probe=%TEMP%\datadog-agent\remote-cache-probe"
if exist "%_probe%" (
  set "_r="
  set /p _r=<"%_probe%"
  if "!_r!"=="ok" exit /b 0
)
curl.exe --silent --output NUL --connect-timeout 2 --max-time 4 "https://buildbarn-frontend-datadog-agent.us1.ddbuild.io/" >nul 2>&1
if not !errorlevel!==0 exit /b 1
if not exist "%TEMP%\datadog-agent" mkdir "%TEMP%\datadog-agent" >nul 2>&1
>"%_probe%" echo ok
exit /b 0
