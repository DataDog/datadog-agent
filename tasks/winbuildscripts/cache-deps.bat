if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing

mkdir \dev\go\src\github.com\DataDog\datadog-agent
cd \dev\go\src\github.com\DataDog\datadog-agent
xcopy /e/s/h/q c:\mnt\*.*
if not exist c:\tmp mkdir c:\tmp
if not exist c:\gomodcache mkdir c:\gomodcache

@echo GOPATH %GOPATH%

pip3 install -r requirements.txt || exit /b 102

inv -e deps || exit /b 103

REM We create this file in c:\tmp first and then move it to c:\mnt.
REM
REM The reason for this is that we're running this in a container in
REM a Gitlab runner. When a job running this gets cancelled, the container
REM keeps running and if it's writing to a file in c:\mnt, the Gitlab
REM runner will be unable to remove that file when trying to clean
REM build dir for any other jobs (until the container exits).
REM Packing it up first and then just moving minimizes this risk.
Powershell -C "7z a c:\tmp\modcache.tar c:\gomodcache"
move c:\tmp\modcache.tar c:\mnt

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF
