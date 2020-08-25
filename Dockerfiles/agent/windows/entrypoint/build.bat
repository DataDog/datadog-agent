if not exist c:\mnt\ goto nomntdir

@echo c:\mnt found, continuing
@echo PARAMS %*

REM don't use `OUTDIR` as an environment variable. It will confuse the VC build
set PKG_OUTDIR=c:\mnt\build-out\%CI_JOB_ID%

mkdir \dev\go\src\github.com\DataDog\datadog-agent\Dockerfiles\agent\windows\entrypoint
if not exist \dev\go\src\github.com\DataDog\datadog-agent\Dockerfiles\agent\windows\entrypoint exit /b 1
cd \dev\go\src\github.com\DataDog\datadog-agent\Dockerfiles\agent\windows\entrypoint || exit /b 2
xcopy /e/s/h/q c:\mnt\Dockerfiles\agent\windows\entrypoint || exit /b 3

@echo PATH %PATH%
@echo VSTUDIO_ROOT %VSTUDIO_ROOT%
@echo TARGET_ARCH %TARGET_ARCH%

call "C:\Program Files (x86)\Microsoft Visual Studio\2017\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
msbuild /p:Configuration=Release /p:Platform=%TARGET_ARCH% || exit /b 4
xcopy \dev\go\src\github.com\DataDog\datadog-agent\Dockerfiles\agent\windows\entrypoint\%TARGET_ARCH%\Release\entrypoint.exe %PKG_OUTDIR%  || exit /b 5

goto :EOF

:nomntdir
@echo directory not mounted, parameters incorrect
exit /b 1
goto :EOF
