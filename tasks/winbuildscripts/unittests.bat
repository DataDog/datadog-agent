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


Powershell -C "c:\mnt\tasks\winbuildscripts\unittests.ps1"
