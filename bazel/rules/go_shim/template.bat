@echo off

set "GOROOT=%cd%\{{goroot}}"
set "GOTOOLCHAIN=local"
set "PATH=%cd%\{{go_dir}};%PATH%"
set "tool=%cd%\{{tool}}"

cd /d "%BUILD_WORKING_DIRECTORY%" || exit /b
"%tool%" %*
