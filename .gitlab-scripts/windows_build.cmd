REM set WIN_CI_PROJECT_DIR=%CD%
REM set WORKON_HOME=%WIN_CI_PROJECT_DIR%


call ridk enable
call "%VCINSTALLDIR%\Common7\Tools\VsDevCmd.bat"

cd %GOPATH%\src\github.com\StackVista\stackstate-agent
echo cd %GOPATH%\src\github.com\StackVista\stackstate-agent

echo git config --global user.email "gitlab@runner.some"
git config --global user.email "gitlab@runner.some"
echo git config --global user.name "Gitlab runner"
git config --global user.name "Gitlab runner"

echo call %WORKON_HOME%\venv\Scripts\activate.bat
call "%WORKON_HOME%\venv\Scripts\activate.bat"
echo ====- pip install
pip install -r requirements.txt
inv -e agent.omnibus-build --skip-sign --log-level debug --skip-deps
