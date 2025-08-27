:: Do not print commands
@echo off
:: Needed for the for loop, otherwise variables are expanded at parse time
setlocal enabledelayedexpansion

:: Parse command line arguments
set "uninstall_flag="

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
        set "uninstall_flag=true"
    )
)

if defined uninstall_flag (
    echo Running APM uninstall command...
    "%installerPath%" remove datadog-apm-library-dotnet
) else (
    goto usage
)
exit /b 0

:: Display usage/help
:usage
echo Datadog IIS Instrumentation
echo.
echo Usage: %0 [options]
echo.
echo Options:
echo   --help^|-h         OPTIONAL Display this message
echo   --uninstall        OPTIONAL Remove installation
echo.
goto :EOF
