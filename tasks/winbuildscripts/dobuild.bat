@echo PARAMS %*
@echo RELEASE_VERSION %RELEASE_VERSION%
@echo MAJOR_VERSION %MAJOR_VERSION%
@echo PY_RUNTIMES %PY_RUNTIMES%
@echo GO_VERSION_CHECK %GO_VERSION_CHECK%

if NOT DEFINED RELEASE_VERSION set RELEASE_VERSION=%~1
if NOT DEFINED MAJOR_VERSION set MAJOR_VERSION=%~2
if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~3
if NOT DEFINED GO_VERSION_CHECK set GO_VERSION_CHECK=%~4

set OMNIBUS_BUILD=agent.omnibus-build
set OMNIBUS_ARGS=--python-runtimes "%PY_RUNTIMES%"

if "%OMNIBUS_TARGET%" == "iot" set OMNIBUS_ARGS=--flavor iot
if "%OMNIBUS_TARGET%" == "dogstatsd" set OMNIBUS_BUILD=dogstatsd.omnibus-build && set OMNIBUS_ARGS=
if "%OMNIBUS_TARGET%" == "agent_binaries" set OMNIBUS_ARGS=%OMNIBUS_ARGS% --agent-binaries
if DEFINED GOMODCACHE set OMNIBUS_ARGS=%OMNIBUS_ARGS% --go-mod-cache %GOMODCACHE%
if DEFINED USE_S3_CACHING set OMNIBUS_ARGS=%OMNIBUS_ARGS% %USE_S3_CACHING%

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

inv -e deps || exit /b 103
if "%GO_VERSION_CHECK%" == "true" (
    inv -e check-go-version || exit /b 104
)

@echo "inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION%"
inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION% || exit /b 105

REM only build MSI for main targets for now.
if "%OMNIBUS_TARGET%" == "main" (
    @echo "inv -e msi.build --major-version %MAJOR_VERSION% --python-runtimes "%PY_RUNTIMES%"
    inv -e msi.build --major-version %MAJOR_VERSION% --python-runtimes "%PY_RUNTIMES%" || exit /b 106
)
