if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*


set TEST_ROOT=c:\buildroot
mkdir %TEST_ROOT%\datadog-agent
if not exist %TEST_ROOT%\datadog-agent exit /b 2
cd %TEST_ROOT%\datadog-agent || exit /b 3
xcopy /e/s/h/q c:\mnt\*.* || exit /b 4

call %TEST_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %TEST_ROOT%\datadog-agent modcache
call %TEST_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %TEST_ROOT%\datadog-agent modcache_tools

Powershell -C "%TEST_ROOT%\datadog-agent\tasks\winbuildscripts\lint.ps1" || exit /b 2

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
