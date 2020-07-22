
@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%
@echo NEW_BUILDER %NEW_BUILDER%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1
if NOT DEFINED NEW_BUILDER set NEW_BUILDER=%~2

cd \dev\go\src\github.com\DataDog\datadog-agent

Powershell -C "c:\dev\go\src\github.com\datadog\datadog-agent\tasks\winbuildscripts\unittests.ps1"
