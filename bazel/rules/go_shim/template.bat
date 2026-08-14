@echo off

set "PATH=%cd%\{{go_dir}};%PATH%"
set "RUNFILES_DIR=%cd%\.."
set "tool=%cd%\{{tool}}"

cd /d "%BUILD_WORKING_DIRECTORY%" || exit /b
"%tool%" %*
