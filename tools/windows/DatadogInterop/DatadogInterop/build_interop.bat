@echo off
setlocal enabledelayedexpansion

set MSBUILD=%1
set VCXPROJ=%2
set OUTPUT=%3

for %%F in (%MSBUILD%) do set MSBUILD=%%~fF
for %%F in (%VCXPROJ%) do set VCXPROJ=%%~fF
for %%F in (%OUTPUT%) do set OUTDIR=%%~dpF
set INTDIR=%OUTDIR%obj\

REM Strip trailing backslash to avoid \" quoting issues with MSBuild
if "%OUTDIR:~-1%"=="\" set OUTDIR=%OUTDIR:~0,-1%
if "%INTDIR:~-1%"=="\" set INTDIR=%INTDIR:~0,-1%

echo === CWD=%CD% 1>&2
echo === MSBUILD=%MSBUILD% 1>&2
echo === VCXPROJ=%VCXPROJ% 1>&2
echo === OUTDIR=%OUTDIR% 1>&2
echo === OUTPUT=%OUTPUT% 1>&2

"%MSBUILD%" "%VCXPROJ%" /p:Configuration=Release /p:Platform="x64" /p:OutDir="%OUTDIR%\\" /p:IntDir="%INTDIR%\\" /verbosity:detailed 1>&2
set BUILD_EXIT=%errorlevel%

echo === BUILD_EXIT=%BUILD_EXIT% 1>&2
dir /s /b "%OUTDIR%" 1>&2

if not exist "%OUTPUT%" (
    echo === OUTPUT NOT FOUND: %OUTPUT% 1>&2
    exit /b 1
)
if %BUILD_EXIT% neq 0 exit /b %BUILD_EXIT%