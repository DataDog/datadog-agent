if not exist c:\mnt\ goto nomntdir
@echo on
@echo c:\mnt found, continuing
@echo PARAMS %*
@echo RELEASE_VERSION %RELEASE_VERSION%

set BUILD_ROOT=c:\mnt
cd %BUILD_ROOT%
call %BUILD_ROOT%\tasks\winbuildscripts\extract-modcache.bat %BUILD_ROOT%\datadog-agent modcache

if NOT DEFINED RELEASE_VERSION set RELEASE_VERSION=%~1

set OMNIBUS_BUILD=omnibus.build
set OMNIBUS_ARGS=%OMNIBUS_ARGS% --target-project installer
set CI_PROJECT_DIR=%BUILD_ROOT%

if DEFINED GOMODCACHE set OMNIBUS_ARGS=%OMNIBUS_ARGS% --go-mod-cache %GOMODCACHE%
if DEFINED USE_S3_CACHING set OMNIBUS_ARGS=%OMNIBUS_ARGS% %USE_S3_CACHING%

SET PATH=%PATH%;%GOPATH%/bin

@echo GOPATH %GOPATH%
@echo PATH %PATH%
@echo VSTUDIO_ROOT %VSTUDIO_ROOT%

call ridk enable
pip3 install -r requirements.txt

@echo "inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --release-version %RELEASE_VERSION%"
inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --release-version %RELEASE_VERSION% || exit /b 1
copy %BUILD_ROOT%\tools\windows\DatadogAgentInstaller\WixSetup\datadog-installer-1-x86_64.msi \omnibus-ruby\pkg\

REM show output package directories (for debugging)
dir \omnibus-ruby\pkg\

REM show output binary directories (for debugging)
dir C:\opt\datadog-installer\

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF


