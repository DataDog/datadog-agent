:: Do not print commands
@echo off
:: Needed for the for loop, otherwise variables are expanded at parse time
setlocal enabledelayedexpansion

:: Parse command line arguments
set "action_flag="

:: Try to get installer path from registry, fallback to default
set "installerPath="
for /f "tokens=2*" %%a in ('reg query "HKLM\SOFTWARE\Datadog\Datadog Agent" /v "InstallPath" 2^>nul') do (
    set "installerPath=%%b\bin\datadog-installer.exe"
)

:: If registry lookup failed, use default path
if not defined installerPath (
    set "installerPath=C:\Program Files\Datadog\Datadog Agent\bin\datadog-installer.exe"
)

for %%A in (%*) do (
    set "arg=%%A"
    if /i "!arg!" == "-h" (
        goto usage
    )
    if /i "!arg!" == "--help" (
        goto usage
    )
    if /i "!arg!" == "--uninstall" (
        if defined action_flag (
            echo Error: Only one action flag can be used at a time.
            goto usage
        )
        set "action_flag=uninstall"
    )
    if /i "!arg!" == "--enable" (
        if defined action_flag (
            echo Error: Only one action flag can be used at a time.
            goto usage
        )
        set "action_flag=enable"
    )
    if /i "!arg!" == "--disable" (
        if defined action_flag (
            echo Error: Only one action flag can be used at a time.
            goto usage
        )
        set "action_flag=disable"
    )
)

:: Run command based on selected flag
if /i "%action_flag%"=="uninstall" (
    echo Running APM uninstall command...
    "%installerPath%" remove datadog-apm-library-dotnet
    exit /b 0
)

if /i "%action_flag%"=="enable" (
    echo Enabling APM instrumentation...
    "%installerPath%" apm instrument iis
    exit /b 0
)

if /i "%action_flag%"=="disable" (
    echo Disabling APM instrumentation...
    "%installerPath%" apm uninstrument iis
    exit /b 0
)

:: No recognized flag
goto usage

:: Display usage/help
:usage
echo Datadog IIS Instrumentation
echo.
echo Usage: %0 [options]
echo.
echo Options:
echo   --help^|-h         OPTIONAL Display this message
echo   --uninstall        OPTIONAL Remove installation
echo   --enable           OPTIONAL Enable instrumentation
echo   --disable          OPTIONAL Enable instrumentation
echo.
goto :EOF
