@echo RELEASE_VERSION %RELEASE_VERSION%
@echo MAJOR_VERSION %MAJOR_VERSION%
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED RELEASE_VERSION set RELEASE_VERSION=nightly
if NOT DEFINED MAJOR_VERSION set MAJOR_VERSION=7
if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES="3"
if NOT DEFINED CI_JOB_ID set CI_JOB_ID=1
if NOT DEFINED TARGET_ARCH set TARGET_ARCH=x64
set NEW_BUILDER=true

REM don't use `OUTDIR` as an environment variable. It will confuse the VC build
set PKG_OUTDIR=c:\mnt\build-out\%CI_JOB_ID%

set OMNIBUS_BUILD=agent.omnibus-build
set OMNIBUS_ARGS=--python-runtimes "%PY_RUNTIMES%"

if "%OMNIBUS_TARGET%" == "iot" set OMNIBUS_ARGS=--iot
if "%OMNIBUS_TARGET%" == "dogstatsd" set OMNIBUS_BUILD=dogstatsd.omnibus-build && set OMNIBUS_ARGS=
if "%OMNIBUS_TARGET%" == "agent_binaries" set OMNIBUS_ARGS=%OMNIBUS_ARGS% --agent-binaries

if not exist \dev\go\src\github.com\DataDog\datadog-agent exit /b 1
cd \dev\go\src\github.com\DataDog\datadog-agent || exit /b 2

SET PATH=%PATH%;%GOPATH%/bin

@echo GOPATH %GOPATH%
@echo PATH %PATH%
@echo VSTUDIO_ROOT %VSTUDIO_ROOT%
@echo TARGET_ARCH %TARGET_ARCH%

REM Equivalent to the "ridk enable" command, but without the exit
if "%TARGET_ARCH%" == "x64" (
    @echo IN x64 BRANCH
    @for /f "delims=" %%x in ('"ruby" --disable-gems -x '%RIDK%' enable') do set "%%x"
)

if "%TARGET_ARCH%" == "x86" (
    @echo IN x86 BRANCH
    REM Use 64-bit toolchain to build gems
    Powershell -C "ridk enable; cd omnibus; bundle install"
)

pip install -r requirements.txt || exit /b 4

inv -e deps --verbose || exit /b 5

@echo "inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION%"
inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION% || exit /b 6

dir \omnibus\pkg

dir \omnibus-ruby\pkg\

if not exist %PKG_OUTDIR% mkdir %PKG_OUTDIR% || exit /b 7
if exist \omnibus-ruby\pkg\*.msi copy \omnibus-ruby\pkg\*.msi %PKG_OUTDIR% || exit /b 8
if exist \omnibus-ruby\pkg\*.zip copy \omnibus-ruby\pkg\*.zip %PKG_OUTDIR% || exit /b 9

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF

