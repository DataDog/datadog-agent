if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1

call %~p0extract-modcache.bat
call %~p0extract-tools-modcache.bat

mkdir \dev\go\src\github.com\DataDog\datadog-agent
if not exist \dev\go\src\github.com\DataDog\datadog-agent exit /b 2
cd \dev\go\src\github.com\DataDog\datadog-agent || exit /b 3
xcopy /e/s/h/q c:\mnt\*.* || exit /b 4


Powershell -C "c:\mnt\tasks\winbuildscripts\sysprobe.ps1" || exit /b 5

REM copy resulting packages to expected location for collection by gitlab.
if not exist c:\mnt\test\kitchen\site-cookbooks\dd-system-probe-check\files\default\tests\ mkdir c:\mnt\test\kitchen\site-cookbooks\dd-system-probe-check\files\default\tests\ || exit /b 6
xcopy /e/s/q \dev\go\src\github.com\DataDog\datadog-agent\test\kitchen\site-cookbooks\dd-system-probe-check\files\default\tests\*.* c:\mnt\test\kitchen\site-cookbooks\dd-system-probe-check\files\default\tests\ || exit /b 7
