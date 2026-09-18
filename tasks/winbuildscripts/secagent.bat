if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

@echo PARAMS %*

set BUILD_ROOT=c:\buildroot
mkdir %BUILD_ROOT%\datadog-agent
if not exist %BUILD_ROOT%\datadog-agent exit /b 2
cd %BUILD_ROOT%\datadog-agent || exit /b 3
robocopy c:\mnt . /e /ndl /nfl /njh /njs /np /xd .cache
:: https://ss64.com/nt/robocopy-exit.html
if %errorlevel% geq 8 exit /b %errorlevel%

call %BUILD_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %BUILD_ROOT%\datadog-agent modcache
call %BUILD_ROOT%\datadog-agent\tasks\winbuildscripts\extract-modcache.bat %BUILD_ROOT%\datadog-agent modcache_tools


Powershell -C "%BUILD_ROOT%\datadog-agent\tasks\winbuildscripts\secagent.ps1" || exit /b 5

REM copy resulting packages to expected location for collection by gitlab.
if not exist c:\mnt\test\new-e2e\tests\security-agent-functional\artifacts\ mkdir c:\mnt\test\new-e2e\tests\security-agent-functional\artifacts\ || exit /b 6
xcopy /e/s/q %BUILD_ROOT%\datadog-agent\test\new-e2e\tests\security-agent-functional\artifacts\*.* c:\mnt\test\new-e2e\tests\security-agent-functional\artifacts\ || exit /b 7
