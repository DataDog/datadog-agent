REM don't let variables escape
@echo off
@setlocal
set DEFAULT_MODCACHE_ROOT="c:\mnt"
set MODCACHE_ROOT=%DEFAULT_MODCACHE_ROOT%
if not "%1" == "" set MODCACHE_ROOT=%1

set MODCACHE_GZ_FILE="%MODCACHE_ROOT%\modcache.tar.gz"
set MODCACHE_TAR_FILE="%MODCACHE_ROOT%\modcache.tar"

if "%GOMODCACHE%" == "" (
    @echo GOMODCACHE environment variable not set, skipping expansion of mod cache files
    goto :endofscript
)
@echo MODCACHE_GZ_FILE %MODCACHE_GZ_FILE% MODCACHE_TAR_FILE %MODCACHE_TAR_FILE% GOMODCACHE %GOMODCACHE%
if exist %MODCACHE_GZ_FILE% (
    @echo Extracting modcache file %MODCACHE_GZ_FILE%
    Powershell -C "7z x %MODCACHE_GZ_FILE% -o%MODCACHE_ROOT%
    dir %MODCACHE_TAR_FILE%
    Powershell -C "7z x %MODCACHE_TAR_FILE% -o%GOMODCACHE%"
    @echo Modcache extracted
) else (
    @echo modcache.tar.gz not found, dependencies will be downloaded
)

:endofscript
if exist %MODCACHE_GZ_FILE% (
    @echo deleting modcache tar.gz %MODCACHE_GZ_FILE%
    del /f %MODCACHE_GZ_FILE%
)
if exist %MODCACHE_TAR_FILE% (
    @echo deleting modcache tar %MODCACHE_TAR_FILE%
    del /f %MODCACHE_TAR_FILE%
)

@endlocal