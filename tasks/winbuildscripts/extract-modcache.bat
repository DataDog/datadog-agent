REM don't let variables escape
@echo off
@setlocal
set DEFAULT_MODCACHE_ROOT="c:\mnt"
set MODCACHE_ROOT=%DEFAULT_MODCACHE_ROOT%
if "%1" == "" (
    goto :usage
)
if "%2" == ""(
    goto :usage
)
if "%GOMODCACHE%" == "" (
    @echo GOMODCACHE environment variable not set, skipping expansion of mod cache files
    goto :endofscript
)

set MODCACHE_ROOT="%1"
set MODCACHE_FILE_ROOT="%2"
set MODCACHE_GZ_FILE="%MODCACHE_ROOT%\%MODCACHE_FILE_ROOT%.tar.gz"
set MODCACHE_TAR_FILE="%MODCACHE_ROOT%\%MODCACHE_FILE_ROOT%.tar"

@echo MODCACHE_GZ_FILE %MODCACHE_GZ_FILE% MODCACHE_TAR_FILE %MODCACHE_TAR_FILE% GOMODCACHE %GOMODCACHE%
if exist %MODCACHE_GZ_FILE% (
    @echo Extracting modcache file %MODCACHE_GZ_FILE%
    Powershell -C "7z x %MODCACHE_GZ_FILE% -o%MODCACHE_ROOT%
    dir %MODCACHE_TAR_FILE%
    REM Use -aoa to allow overwriting existing files
    REM This shouldn't have any negative impact: since modules are
    REM stored per version and hash, files that get replaced will
    REM get replaced by the same files
    Powershell -C "7z x %MODCACHE_TAR_FILE% -o%GOMODCACHE% -aoa"
    @echo Modcache extracted
) else (
    @echo %MODCACHE_GZ_FILE% not found, dependencies will be downloaded
)
goto :endofscript

:usage
@echo usage
@echo extract-modcache <build root> <filename>
goto :eof

:endofscript
if exist %MODCACHE_GZ_FILE% (
    @echo deleting modcache tar.gz %MODCACHE_GZ_FILE%
    del /f %MODCACHE_GZ_FILE%
)
if exist %MODCACHE_TAR_FILE% (
    @echo deleting modcache tar %MODCACHE_TAR_FILE%
    del /f %MODCACHE_TAR_FILE%
)
goto :EOF



@endlocal