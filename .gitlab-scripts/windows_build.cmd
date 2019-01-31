REM set WIN_CI_PROJECT_DIR=%CD%
REM set WORKON_HOME=%WIN_CI_PROJECT_DIR%


echo call %WORKON_HOME%\venv\Scripts\activate.bat
call %WORKON_HOME%\venv\Scripts\activate.bat
call ridk enable

set

dir

cd %GOPATH%\src\github.com\StackVista\stackstate-agent
echo cd %GOPATH%\src\github.com\StackVista\stackstate-agent

echo git config --global user.email "gitlab@runner.some"
git config --global user.email "gitlab@runner.some"
echo git config --global user.name "Gitlab runner"
git config --global user.name "Gitlab runner"
inv -e agent.omnibus-build --skip-sign --log-level debug --skip-deps
