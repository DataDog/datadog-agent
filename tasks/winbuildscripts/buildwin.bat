if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing
@echo PARAMS %*

set RELEASE_VERSION=%1
set MAJOR_VERSION=%2

REM don't use `OUTDIR` as an environment variable. It will confuse the VC build
set PKG_OUTDIR=c:\mnt\build-out\%CI_JOB_ID%

set OMNIBUS_BUILD="agent.omnibus-build"
set OMNIBUS_ARGS=

if "%OMNIBUS_TARGET%" == "puppy" set OMNIBUS_ARGS="--puppy"
if "%OMNIBUS_TARGET%" == "dogstatsd" set OMNIBUS_BUILD="dogstatsd.omnibus-build"
if "%OMNIBUS_TARGET%" == "cf_buildpack" set OMNIBUS_ARGS="--cf-windows"

mkdir \dev\go\src\github.com\DataDog\datadog-agent 
if not exist \dev\go\src\github.com\DataDog\datadog-agent exit /b 1
cd \dev\go\src\github.com\DataDog\datadog-agent || exit /b 2
xcopy /e/s/h/q c:\mnt\*.* || exit /b 3
inv -e deps --verbose --dep-vendor-only --no-checks || exit /b 4

inv -e %OMNIBUS_BUILD% %OMNIBUS_ARGS% --skip-deps --major-version %MAJOR_VERSION% --release-version %RELEASE_VERSION% || exit /b 5

dir \omnibus\pkg

dir \omnibus-ruby\pkg\

if not exist %PKG_OUTDIR% mkdir %PKG_OUTDIR% || exit /b 6
if exist \omnibus-ruby\pkg\*.msi copy \omnibus-ruby\pkg\*.msi %PKG_OUTDIR% || exit /b 7
if exist \omnibus-ruby\pkg\*.zip copy \omnibus-ruby\pkg\*.zip %PKG_OUTDIR% || exit /b 8

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF


