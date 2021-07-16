REM set WIN_CI_PROJECT_DIR=%CD%
REM set WORKON_HOME=%WIN_CI_PROJECT_DIR%
set VCINSTALLDIR=C:\\Program Files (x86)\\Microsoft Visual Studio\\2017\\Community
echo ====- cleaning existing venv
IF EXIST c:\deps GOTO C_DEPS_EXIST
call %WIN_CI_PROJECT_DIR%\.gitlab-scripts\pkg_configs.cmd
:C_DEPS_EXIST
if exist .omnibus rd /s/q .omnibus
mkdir .omnibus\pkg
if exist \omnibus-ruby rd /s/q \omnibus-ruby
if exist %OMNIBUS_BASE_DIR_WIN% rd /s/q %OMNIBUS_BASE_DIR_WIN%
if exist \opt\stackstate-agent rd /s/q \opt\stackstate-agent
if exist %GOPATH%\src\github.com\StackVista\stackstate-agent rd /s/q %GOPATH%\src\github.com\StackVista\stackstate-agent
REM mkdir %GOPATH%\src\github.com\StackVista\stackstate-agent
REM xcopy /q/h/e/s * %GOPATH%\src\github.com\StackVista\stackstate-agent
mkdir c:\gopath\src\github.com\StackVista\
mklink /J %GOPATH%\src\github.com\StackVista\stackstate-agent %WIN_CI_PROJECT_DIR%
cd %GOPATH%\src\github.com\StackVista\stackstate-agent
echo ====- cleaning existing venv
rmdir /q /s %GOPATH%\src\github.com\StackVista\stackstate-agent\venv
echo ====- creating venv with mkvirtualenv venv
call mkvirtualenv venv
cd %GOPATH%\src\github.com\StackVista\stackstate-agent
echo ====- installing requirements.txt from repo root
pip install -r requirements.txt
dir %GOPATH%\src\github.com\StackVista\stackstate-agent\venv\Lib\site-packages

