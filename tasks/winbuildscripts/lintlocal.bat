@echo PARAMS %*
@echo PY_RUNTIMES %PY_RUNTIMES%

if NOT DEFINED PY_RUNTIMES set PY_RUNTIMES=%~1


Powershell -C "%~dp0lint.ps1"
