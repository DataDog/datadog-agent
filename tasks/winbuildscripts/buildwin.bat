if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

mkdir \dev\go\src\github.com\DataDog\datadog-agent
if not exist \dev\go\src\github.com\DataDog\datadog-agent exit /b 1
cd \dev\go\src\github.com\DataDog\datadog-agent || exit /b 2
xcopy /e/s/h/q c:\mnt\*.* || exit /b 3

REM
REM after copying files in from the host, execute the build
REM using `dobuild.bat`
REM
call %~p0dobuild.bat %* || exit /b %ERRORLEVEL%

REM show output directories (for debugging)
dir \omnibus\pkg

dir \omnibus-ruby\pkg\

REM copy resulting packages to expected location for collection by gitlab.
if not exist %PKG_OUTDIR% mkdir %PKG_OUTDIR% || exit /b 7
if exist \omnibus-ruby\pkg\*.msi copy \omnibus-ruby\pkg\*.msi %PKG_OUTDIR% || exit /b 8
if exist \omnibus-ruby\pkg\*.zip copy \omnibus-ruby\pkg\*.zip %PKG_OUTDIR% || exit /b 9

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF


