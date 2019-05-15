REM set WIN_CI_PROJECT_DIR=%CD%
REM set WORKON_HOME=%WIN_CI_PROJECT_DIR%

set

dir

echo call %WORKON_HOME%\venv\Scripts\activate.bat
call %WORKON_HOME%\venv\Scripts\activate.bat

IF EXIST %GOPATH%\src\github.com\StackVista\stackstate-agent\vendor GOTO VENDOR_EXIST

inv -e deps -v

:VENDOR_EXIST
