@echo off
setlocal EnableDelayedExpansion
>nul chcp 65001

:: Bazel hashes `SystemRoot` verbatim, so pin it to CI's de facto casing to avoid cache misses on dev machines
set "SystemRoot=%SystemRoot:WINDOWS=Windows%"

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if defined BAZEL_REAL if "%BAZELISK_SKIP_WRAPPER%"=="true" goto :bazelisk_ok
>&2 type "%~dp0bazelisk.md"
exit /b 2
:bazelisk_ok

:: Ensure `XDG_CACHE_HOME` denotes a directory
if not defined DOTNET_RUNNING_IN_CONTAINER >nul 2>&1 sc query CExecSvc && set DOTNET_RUNNING_IN_CONTAINER=1
if not exist "%XDG_CACHE_HOME%" (
  if defined CI goto :error_xdg_cache_home_must_exist
  if defined DOTNET_RUNNING_IN_CONTAINER (
    >&2 echo 💡 To persist caches across restarts, please set XDG_CACHE_HOME pointing to a mounted directory, e.g.:
    >&2 echo     docker.exe run --env=XDG_CACHE_HOME=C:\cache --volume="$HOME\.cache:C:\cache" ...
  )
)

:: Ensure `bazel` & managed toolchains honor `XDG_CACHE_HOME`
set "extra_args="
if defined XDG_CACHE_HOME (
  set "XDG_CACHE_HOME=!XDG_CACHE_HOME:/=\!"
  if "!XDG_CACHE_HOME:~1,2!" neq ":\" if "!XDG_CACHE_HOME:~0,2!" neq "\\" goto :error_xdg_cache_home_must_be_absolute
  set "GOCACHE=!XDG_CACHE_HOME!\go-build"
  set "GOLANGCI_LINT_CACHE=!XDG_CACHE_HOME!\golangci-lint"
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
  if defined CI if not defined GITHUB_ACTIONS (
    if not defined BUILDBARN_ID_TOKEN goto :error_buildbarn_id_token_must_be_set
    set "extra_args=!extra_args! --config=ci --config=cache:frontend"
  )
) else (
  :: Without XDG_CACHE_HOME, fall back Go caches to official defaults so Go repo rules work under strict repo_env
  if not defined GOCACHE set "GOCACHE=%LOCALAPPDATA%\go-build"
  if not defined GOMODCACHE (
    if defined GOPATH (for /f "tokens=1 delims=;" %%i in ("%GOPATH%") do set "gp=%%i") else set "gp=%USERPROFILE%\go"
    set "GOMODCACHE=!gp!\pkg\mod"
    set "gp="
  )
)

:: Local developer cache and download mirror selection (CI selects its own above).
if not defined CI (
  call :remote_cache_select
  set "_adms_options=%*"
  call :adms_select
)

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

:: Check 8.3 short names are enabled, or warn with instructions
set "more_than_8dot3_chars=%TEMP%\123456789.1234"
2>nul del /f /q "!more_than_8dot3_chars!"
>"!more_than_8dot3_chars!" type nul
for %%i in ("!more_than_8dot3_chars!") do if "%%~nxi"=="%%~snxi" (
  >&2 echo 🟡 For `bazel` to spawn processes from long paths, please enable 8.3 short names on %%~di:
  >&2 echo     fsutil 8dot3name set %%~di 0
)

:: Check symlink creation privilege (required by rules_python bootstrap on Windows)
set "_sl_probe=%TEMP%\bazel_sl_probe_%RANDOM%_%RANDOM%"
set "_sl_target=%TEMP%\bazel_sl_target_%RANDOM%_%RANDOM%"
>"!_sl_target!" type nul
2>nul mklink "!_sl_probe!" "!_sl_target!" >nul
set "_sl_rc=!errorlevel!"
2>nul del /f /q "!_sl_probe!" "!_sl_target!"
if !_sl_rc! neq 0 (
  >&2 echo 🔴 For `bazel` to work properly, please enable Windows Developer Mode, which grants symlink creation privilege:
  >&2 echo     Settings ^> System ^> Advanced ^> For developers ^> Developer Mode
  exit /b 2
)

set "args=%*"
if defined args if defined extra_args call :insert_extra_args
"%BAZEL_REAL%" !startup_options! !args!
exit /b !errorlevel!

:error_buildbarn_id_token_must_be_set
>&2 echo 🔴 BUILDBARN_ID_TOKEN must be populated in CI
exit /b 2

:error_xdg_cache_home_must_exist
>&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote a directory in CI
exit /b 2

:error_xdg_cache_home_must_be_absolute
>&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote an absolute path
exit /b 2

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
:: An explicit cache config on the command line, or an rc-file opt-out, wins.
echo %* | findstr /C:"--config=cache" /C:"--config=no-remote-cache" >nul && goto :eof
call :rc_opts_out && goto :eof
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

:: True (exit 0) when a user rc file opts out of the remote cache. The wrapper
:: injects --config=cache on the command line, which would otherwise beat an
:: rc-level --config=no-remote-cache (command-line options win over rc ones).
:rc_opts_out
for %%R in ("%~dp0..\user.bazelrc" "%USERPROFILE%\.bazelrc") do (
  if exist "%%~R" findstr /R /C:"^[^#]*config=no-remote-cache" "%%~R" >nul 2>&1 && exit /b 0
)
exit /b 1

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

:: Reachability probe with asymmetric caching (mirrors remote-cache-select.sh).
:: Any HTTPS response (incl. gRPC's 415) counts as reachable; only a
:: connection/TLS failure counts as unreachable. A positive result is sticky
:: until %TEMP% is cleared; a negative result is cached for 60s so a VPN
:: reconnect is picked up quickly without re-probing on every build.
:remote_cache_reachable
set "_dir=%TEMP%\datadog-agent"
set "_probe=%_dir%\remote-cache-probe"
if exist "%_probe%" (
  set "_r="
  set /p _r=<"%_probe%"
  if "!_r!"=="ok" exit /b 0
  if "!_r!"=="no" (
    set "_age="
    for /f %%A in ('powershell -NoProfile -Command "[int]((Get-Date)-(Get-Item '%_probe%').LastWriteTime).TotalSeconds" 2^>nul') do set "_age=%%A"
    if defined _age if !_age! lss 60 exit /b 1
  )
)
if not exist "%_dir%" mkdir "%_dir%" >nul 2>&1
curl.exe --silent --output NUL --connect-timeout 2 --max-time 4 "https://buildbarn-frontend-datadog-agent.us1.ddbuild.io/" >nul 2>&1
if !errorlevel! neq 0 (
  >"%_probe%" echo no
  exit /b 1
)
>"%_probe%" echo ok
exit /b 0

:: ADMS download mirror auto-selection. DD_BAZEL_ADMS=auto (default) | on | off.
:: Explicit downloader settings and ADMS configs take precedence over injection.
:adms_select
call :adms_options_configured && goto :eof
call :adms_rc_configured && goto :eof
if not defined DD_BAZEL_ADMS set "DD_BAZEL_ADMS=auto"
if /i "%DD_BAZEL_ADMS%"=="off" goto :eof
if /i "%DD_BAZEL_ADMS%"=="on" goto :adms_enable
if /i not "%DD_BAZEL_ADMS%"=="auto" (
  >&2 echo 🔴 Unknown DD_BAZEL_ADMS=%DD_BAZEL_ADMS%, expected auto^|on^|off
  goto :eof
)
call :adms_reachable || goto :eof
:adms_enable
if defined extra_args (set "extra_args=!extra_args! --config=adms") else set "extra_args=--config=adms"
goto :eof

:adms_options_configured
set "_adms_remaining=!_adms_options!"
set "_adms_next_config="
:adms_next_option
if not defined _adms_remaining exit /b 1
set "_adms_arg="
for /f "tokens=1*" %%A in ("!_adms_remaining!") do (
  set "_adms_arg=%%~A"
  set "_adms_remaining=%%B"
)
if not defined _adms_arg exit /b 1
if "!_adms_arg!"=="--" exit /b 1
if "!_adms_arg!"=="--config=adms" exit /b 0
if "!_adms_arg!"=="--config=dd-internal" exit /b 0
if "!_adms_arg!"=="--downloader_config" exit /b 0
if "!_adms_arg:~0,20!"=="--downloader_config=" exit /b 0
if defined _adms_next_config (
  if "!_adms_arg!"=="adms" exit /b 0
  if "!_adms_arg!"=="dd-internal" exit /b 0
)
set "_adms_next_config="
if "!_adms_arg!"=="--config" set "_adms_next_config=1"
goto :adms_next_option

:adms_rc_configured
for %%R in ("%~dp0..\user.bazelrc" "%USERPROFILE%\.bazelrc") do (
  if exist "%%~R" for /f "usebackq tokens=1,*" %%S in ("%%~R") do (
    set "_adms_scope=%%S"
    if not "!_adms_scope:~0,1!"=="#" if "!_adms_scope::=!"=="!_adms_scope!" (
      rem Strip inline comments; the prefix preserves empty text before '#'.
      set "_adms_options=%%T"
      for /f "tokens=1 delims=#" %%L in ("x!_adms_options!") do set "_adms_options=%%L"
      set "_adms_options=!_adms_options:~1!"
      call :adms_options_configured && exit /b 0
    )
  )
)
exit /b 1

:: ADMS reachability: expire both results after 60s (no downloader fallback).
:adms_reachable
set "_adms_dir=%TEMP%\datadog-agent"
set "_adms_probe=%_adms_dir%\adms-probe"
if exist "%_adms_probe%" (
  set "_adms_result="
  set /p _adms_result=<"%_adms_probe%"
  set "_adms_age="
  for /f %%A in ('powershell -NoProfile -Command "[int]((Get-Date)-(Get-Item -LiteralPath $env:_adms_probe).LastWriteTime).TotalSeconds" 2^>nul') do set "_adms_age=%%A"
  if defined _adms_age if !_adms_age! lss 60 (
    if "!_adms_result!"=="ok" exit /b 0
    if "!_adms_result!"=="no" exit /b 1
  )
)
if not exist "%_adms_dir%" mkdir "%_adms_dir%" >nul 2>&1
:: The root returns HTTP 400; probe a registry resource and reject HTTP errors.
curl.exe --silent --fail --output NUL --connect-timeout 2 --max-time 4 "https://depot-read-api-bzl.us1.ddbuild.io/bcr.bazel.build/bazel_registry.json" >nul 2>&1
if !errorlevel! neq 0 (
  >"%_adms_probe%" echo no
  exit /b 1
)
>"%_adms_probe%" echo ok
exit /b 0
