if not exist c:\mnt\ goto nomntdir
@echo on
@echo c:\mnt found, continuing
@echo PARAMS %*
@echo RELEASE_VERSION %RELEASE_VERSION%

set BUILD_ROOT=c:\buildroot
set REPO_ROOT=%BUILD_ROOT%\datadog-agent
mkdir %REPO_ROOT%
if not exist %REPO_ROOT% exit /b 2
cd %REPO_ROOT% || exit /b 3
xcopy /e/s/h/q c:\mnt\*.* || exit /b 4

call %BUILD_ROOT%\tasks\winbuildscripts\extract-modcache.bat %REPO_ROOT% modcache

set OMNIBUS_BUILD=omnibus.build
@rem OMNIBUS_TARGET is also used in the C# code to only produce the .cmd for the Datadog Installer (instead of for both the Agent installer and the Datadog installer).
@rem It's not strictly needed, as we will only invoke the .cmd for the Datadog Installer in the invoke task build-installer, but it's a good practice to be consistent.
set OMNIBUS_TARGET=installer
set OMNIBUS_ARGS=%OMNIBUS_ARGS% --target-project %OMNIBUS_TARGET%

if DEFINED GOMODCACHE set OMNIBUS_ARGS=%OMNIBUS_ARGS% --go-mod-cache %GOMODCACHE%
if DEFINED USE_S3_CACHING set OMNIBUS_ARGS=%OMNIBUS_ARGS% %USE_S3_CACHING%

SET PATH=%PATH%;%GOPATH%/bin
REM AGENT_MSI_OUTDIR is always overridden in msi.py

@echo GOPATH %GOPATH%
@echo PATH %PATH%
@echo VSTUDIO_ROOT %VSTUDIO_ROOT%

call ridk enable
pip3 install -r requirements.txt

@echo "inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --release-version %RELEASE_VERSION%"
inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --release-version %RELEASE_VERSION% || exit /b 1
inv -e msi.build-installer || exit /b 2

Powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 --package 'datadog-installer'"

REM show output package directories (for debugging)
dir \omnibus-ruby\pkg\
dir C:\opt\datadog-installer\
dir %REPO_ROOT%\omnibus\pkg\

REM copy resulting packages to expected location for collection by gitlab.
if not exist c:\mnt\omnibus\pkg\ mkdir c:\mnt\omnibus\pkg\ || exit /b 5
copy %REPO_ROOT%\omnibus\pkg\* c:\mnt\omnibus\pkg\ || exit /b 6
REM Save the installer.exe for bootstrapping
copy C:\opt\datadog-installer\datadog-installer.exe c:\mnt\omnibus\pkg\datadog-installer-%RELEASE_VERSION%-1-x86_64.exe || exit /b 7

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF
