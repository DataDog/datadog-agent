if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1

Powershell -C "c:\mnt\tasks\winbuildscripts\junitupload.ps1"

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
