
@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1

cd \dev\go\src\github.com\DataDog\datadog-agent

Powershell -C "c:\dev\go\src\github.com\datadog\datadog-agent\tasks\winbuildscripts\unittests.ps1"
