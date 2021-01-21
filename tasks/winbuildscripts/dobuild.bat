@echo PARAMS %*
@echo RELEASE_VERSION %RELEASE_VERSION%
@echo MAJOR_VERSION %MAJOR_VERSION%
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED RELEASE_VERSION set RELEASE_VERSION=%~1
if NOT DEFINED MAJOR_VERSION set MAJOR_VERSION=%~2
if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~3

REM don't use `OUTDIR` as an environment variable. It will confuse the VC build
set PKG_OUTDIR=c:\mnt\build-out\%CI_JOB_ID%

set OMNIBUS_BUILD=agent.omnibus-build
set OMNIBUS_ARGS=--python-runtimes "%PY_RUNTIMES%"

if "%OMNIBUS_TARGET%" == "iot" set OMNIBUS_ARGS=--iot
if "%OMNIBUS_TARGET%" == "dogstatsd" set OMNIBUS_BUILD=dogstatsd.omnibus-build && set OMNIBUS_ARGS=
if "%OMNIBUS_TARGET%" == "agent_binaries" set OMNIBUS_ARGS=%OMNIBUS_ARGS% --agent-binaries

SET PATH=%PATH%;%GOPATH%/bin

@echo GOPATH %GOPATH%
@echo PATH %PATH%
@echo VSTUDIO_ROOT %VSTUDIO_ROOT%
@echo TARGET_ARCH %TARGET_ARCH%

REM Section to pre-install libyajl2 gem with fix for gcc10 compatibility
Powershell -C "ridk enable; ./tasks/winbuildscripts/libyajl2_install.ps1"


if "%TARGET_ARCH%" == "x64" (
    @echo IN x64 BRANCH
    call ridk enable
)

if "%TARGET_ARCH%" == "x86" (
    @echo IN x86 BRANCH
    REM Use 64-bit toolchain to build gems
    Powershell -C "ridk enable; cd omnibus; bundle install"
)

if not exist \dev\go\src\github.com\DataDog\datadog-agent exit /b 100
cd \dev\go\src\github.com\DataDog\datadog-agent || exit /b 101


pip3 install -r requirements.txt || exit /b 102

inv -e deps --verbose || exit /b 103

@echo "inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION%"
inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION% || exit /b 104
