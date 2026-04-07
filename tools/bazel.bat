@echo off
setlocal EnableDelayedExpansion
>nul chcp 65001

:: Check `bazelisk` properly bootstraps `bazel` or fail with instructions
if defined BAZEL_REAL if "%BAZELISK_SKIP_WRAPPER%"=="true" goto :bazelisk_ok
>&2 type "%~dp0bazelisk.md"
exit /b 2
:bazelisk_ok

:: Download credential helper if not already present
set "credential_helper_dir=%~dp0..\.credential-helper"
set "credential_helper_bin=%credential_helper_dir%\credential-helper.exe"
if not exist "%credential_helper_bin%" (
  if not exist "%credential_helper_dir%" mkdir "%credential_helper_dir%"
  set "credential_helper_version=v0.0.9"
  set "credential_helper_url=https://github.com/tweag/credential-helper/releases/download/!credential_helper_version!/credential_helper_windows_amd64.exe"
  set "credential_helper_expected_hash=efa1d39972088e437f92539600a3d53694f19cb6e93d6bf4bdbe4ff47c79b5fa"
  curl -fsSL -o "%credential_helper_bin%" "!credential_helper_url!" 2>nul
  if !errorlevel! neq 0 (
    >&2 echo Warning: failed to download credential helper
    del /q "%credential_helper_bin%" 2>nul
    goto :credential_helper_done
  )
  for /f "tokens=*" %%h in ('certutil -hashfile "%credential_helper_bin%" SHA256 ^| findstr /v "hash CertUtil"') do set "credential_helper_actual_hash=%%h"
  set "credential_helper_actual_hash=!credential_helper_actual_hash: =!"
  if /i "!credential_helper_actual_hash!" neq "!credential_helper_expected_hash!" (
    >&2 echo Warning: credential helper hash mismatch
    del /q "%credential_helper_bin%" 2>nul
  )
)
:credential_helper_done

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

:: Ensure `bazel` & managed toolchains honor `XDG_CACHE_HOME` as per https://wiki.archlinux.org/title/XDG_Base_Directory
set "extra_args="
if defined XDG_CACHE_HOME (
  set "XDG_CACHE_HOME=!XDG_CACHE_HOME:/=\!"
  if "!XDG_CACHE_HOME:~1,2!" neq ":\" if "!XDG_CACHE_HOME:~0,2!" neq "\\" (
    >&2 echo 🔴 XDG_CACHE_HOME ^(!XDG_CACHE_HOME!^) must denote an absolute path!
    exit /b 2
  )
  :: https://pkg.go.dev/cmd/go#hdr-Build_and_test_caching
  set "GOCACHE=%XDG_CACHE_HOME%\go-build"
  :: https://wiki.archlinux.org/title/XDG_Base_Directory#Partial
  set "GOMODCACHE=%XDG_CACHE_HOME%\go\mod"
  :: https://pip.pypa.io/en/stable/topics/caching/#default-paths
  set "PIP_CACHE_DIR=%XDG_CACHE_HOME%\pip"
  :: https://github.com/bazelbuild/bazel/issues/27808
  set "bazel_home=%XDG_CACHE_HOME%\bazel"
  set bazel_home_startup_option="--output_user_root=!bazel_home!"
  set extra_args="--disk_cache=!bazel_home!\disk-cache"
  :: https://github.com/bazelbuild/bazel/issues/26384
  for %%i in ("%~dp0..\.cache") do if "!XDG_CACHE_HOME!" == "%%~fi" set "extra_args=!extra_args! --repo_contents_cache="
  if defined CI if not defined GITHUB_ACTIONS set "extra_args=!extra_args! --config=ci"
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

if defined BUILDBARN_ID_TOKEN set "BUILDBARN_BEARER_TOKEN=Bearer !BUILDBARN_ID_TOKEN!"

set "args=%*"
if defined args if defined extra_args call :insert_extra_args
:: Prevent rules_android from loading a system Android SDK - TODO(regis): replace with --experimental_strict_repo_env
set "ANDROID_HOME="
"%BAZEL_REAL%" !bazel_home_startup_option! !args!
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
