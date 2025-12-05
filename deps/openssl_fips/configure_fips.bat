@echo off
setlocal enabledelayedexpansion

set "DESTDIR="
set "EMBEDDED_SSL_DIR="

:parse_args
if "%~1"=="" goto :run
if /i "%~1"=="--destdir" (
    set "DESTDIR=%~2"
    shift
    shift
    goto :parse_args
)
if /i "%~1"=="--embedded_ssl_dir" (
    set "EMBEDDED_SSL_DIR=%~2"
    shift
    shift
    goto :parse_args
)
echo Unknown option: %~1
echo Usage: %~nx0 --destdir ^<directory^> [--embedded_ssl_dir ^<directory^>]
exit /b 1

:run
if "%DESTDIR%"=="" (
    echo Error: --destdir is required
    echo Usage: %~nx0 --destdir ^<directory^> [--embedded_ssl_dir ^<directory^>]
    exit /b 1
)

set "SCRIPT_DIR=%~dp0"

if "%EMBEDDED_SSL_DIR%"=="" (
    powershell -ExecutionPolicy Bypass -File "%SCRIPT_DIR%configure_fips.ps1" -destdir "%DESTDIR%"
) else (
    powershell -ExecutionPolicy Bypass -File "%SCRIPT_DIR%configure_fips.ps1" -destdir "%DESTDIR%" -embedded_ssl_dir "%EMBEDDED_SSL_DIR%"
)

