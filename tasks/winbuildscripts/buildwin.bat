if not exist c:\mnt\ goto nomntdir
@echo on
@echo c:\mnt found, continuing

set BUILD_ROOT=c:\buildroot
mkdir %BUILD_ROOT%\datadog-agent
if not exist %BUILD_ROOT%\datadog-agent exit /b 2
cd %BUILD_ROOT%\datadog-agent || exit /b 3
xcopy /e/s/h/q c:\mnt\*.* || exit /b 4

call %BUILD_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %BUILD_ROOT%\datadog-agent modcache

REM
REM after copying files in from the host, execute the build
REM using `dobuild.bat`
REM
call %BUILD_ROOT%\datadog-agent\tasks\winbuildscripts\dobuild.bat %*
if not %ERRORLEVEL% == 0 exit /b %ERRORLEVEL%

REM show output package directories (for debugging)
dir \omnibus-ruby\pkg\

dir %BUILD_ROOT%\datadog-agent\omnibus\pkg\

REM copy resulting packages to expected location for collection by gitlab.
if not exist c:\mnt\omnibus\pkg\ mkdir c:\mnt\omnibus\pkg\ || exit /b 5
copy %BUILD_ROOT%\datadog-agent\omnibus\pkg\* c:\mnt\omnibus\pkg\ || exit /b 6

REM show output binary directories (for debugging)
dir C:\opt\datadog-agent\bin\agent\

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF


