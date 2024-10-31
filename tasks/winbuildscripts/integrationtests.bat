if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*

set TEST_ROOT=c:\test-root
mkdir %TEST_ROOT%\datadog-agent
cd %TEST_ROOT%\datadog-agent
xcopy /e/s/h/q c:\mnt\*.*

call %TEST_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %TEST_ROOT%\datadog-agent modcache
call %TEST_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %TEST_ROOT%\datadog-agent modcache_tools

Powershell -C "%TEST_ROOT%\datadog-agent\tasks\winbuildscripts\integrationtests.ps1" || exit /b 2

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
