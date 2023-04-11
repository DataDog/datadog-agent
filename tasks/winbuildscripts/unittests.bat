if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1

call %~p0extract-modcache.bat
call %~p0extract-tools-modcache.bat

mkdir \dev\go\src\github.com\DataDog\datadog-agent
cd \dev\go\src\github.com\DataDog\datadog-agent
xcopy /e/s/h/q c:\mnt\*.*

REM Setup root certificates before tests
Powershell -C "c:\mnt\tasks\winbuildscripts\setup_certificates.ps1" || exit /b 2

Powershell -C "c:\mnt\tasks\winbuildscripts\unittests.ps1" || exit /b 3

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
