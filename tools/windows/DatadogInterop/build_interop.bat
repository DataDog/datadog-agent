@echo off
setlocal

for %%F in (%MSBUILD%) do set MSBUILD=%%~fF
for %%F in (%VCXPROJ%) do set VCXPROJ=%%~fF
for %%F in (%OUTPUT%) do set OUTPUT=%%~fF
for %%F in (%OUTPUT%) do set OUTDIR=%%~dpF
set INTDIR=%OUTDIR%obj\

if "%OUTDIR:~-1%"=="\" set OUTDIR=%OUTDIR:~0,-1%
if "%INTDIR:~-1%"=="\" set INTDIR=%INTDIR:~0,-1%

:: Point MSBuild at the hermetic MSVC, Windows SDK and VC project system from
:: @toolchains_msvc. Each root is recovered from a marker file staged inside it.
for %%F in (%VCTOOLS_VERSION_FILE%) do set "MSVC_ROOT=%%~dpF"
for %%F in (%WINSDK_MARKER%) do set "WINSDK_ROOT=%%~dpF"
for %%F in (%VCTARGETS_MARKER%) do set "VCTARGETS_PATH=%%~dpF"
:: Force absolute roots; relative paths break Tracker's CL.exe lookup.
for %%F in ("%MSVC_ROOT%.") do set "MSVC_ROOT=%%~fF\"
for %%F in ("%WINSDK_ROOT%.") do set "WINSDK_ROOT=%%~fF\"
for %%F in ("%VCTARGETS_PATH%.") do set "VCTARGETS_PATH=%%~fF\"
set /p VCTOOLS_VERSION=<%VCTOOLS_VERSION_FILE%

for %%F in ("%VCTARGETS_PATH:~0,-1%") do set "VC_TOOLSET_DIR=%%~nxF"
set "VC_TOOLSET=_%VC_TOOLSET_DIR:~1%"

for /d %%D in ("%WINSDK_ROOT%Include\10.0.*") do set "WINSDK_PLATFORM_VERSION=%%~nxD"

set "CL_DIR=%MSVC_ROOT%Tools\bin\Hostx64\x64"
set "WINSDK_BIN=%WINSDK_ROOT%bin\%WINSDK_PLATFORM_VERSION%\x64"
:: Tracker/link look up CL.exe and rc.exe on PATH even when the MSBuild props
:: know the full tool paths.
set "PATH=%CL_DIR%;%WINSDK_BIN%;%PATH%"

set "response_file=%OUTDIR%\msbuild.rsp"
echo. > "%response_file%"
:: Without DisableRegistryUse, the VC props fall back to registry lookups and a
:: host Visual Studio silently wins over everything set below.
echo "/p:DisableRegistryUse=true" >> "%response_file%"
:: The VC props expect these roots to keep their trailing separator, and MSBuild reads
:: \" in a response file as an escaped quote, so the final backslash has to be doubled.
echo "/p:VCTargetsPath=%VCTARGETS_PATH%\" >> "%response_file%"
echo "/p:VCInstallDir%VC_TOOLSET%=%MSVC_ROOT%\" >> "%response_file%"
echo "/p:VCToolsInstallDir%VC_TOOLSET%=%MSVC_ROOT%Tools\\" >> "%response_file%"
echo "/p:VCToolsRedistInstallDir%VC_TOOLSET%=%MSVC_ROOT%Redist\\" >> "%response_file%"
echo "/p:VCToolsVersion=%VCTOOLS_VERSION%" >> "%response_file%"
echo "/p:WindowsSdkDir_10=%WINSDK_ROOT%\" >> "%response_file%"
echo "/p:UniversalCRTSdkDir_10=%WINSDK_ROOT%\" >> "%response_file%"
echo "/p:WindowsTargetPlatformVersion=%WINSDK_PLATFORM_VERSION%" >> "%response_file%"
echo "/p:WindowsSdkDir=%WINSDK_ROOT%\" >> "%response_file%"
echo "/p:UCRTContentRoot=%WINSDK_ROOT%\" >> "%response_file%"
:: The SDK's DesignTime UAP.props is not part of the staged tree.
:: Semicolons must be escaped as %%3B, otherwise MSBuild reads them as switch separators.
set "WINSDK_INC=%WINSDK_ROOT%Include\%WINSDK_PLATFORM_VERSION%"
echo "/p:WindowsSDK_IncludePath=%WINSDK_INC%\um%%3B%WINSDK_INC%\shared%%3B%WINSDK_INC%\winrt%%3B%WINSDK_INC%\cppwinrt" >> "%response_file%"
echo "/p:WindowsSDK_LibraryPath_x64=%WINSDK_ROOT%Lib\%WINSDK_PLATFORM_VERSION%\um\x64" >> "%response_file%"
echo "/p:WindowsSDK_ExecutablePath_x64=%WINSDK_ROOT%bin\%WINSDK_PLATFORM_VERSION%\x64" >> "%response_file%"
echo "/p:PreferredToolArchitecture=x64" >> "%response_file%"
:: Spectre-mitigated CRT is not staged in the hermetic toolchain.
echo "/p:SpectreMitigation=false" >> "%response_file%"
echo "/p:Configuration=Release" >> "%response_file%"
echo "/p:Platform=x64" >> "%response_file%"
echo "/p:OutDir=%OUTDIR%\\" >> "%response_file%"
echo "/p:IntDir=%INTDIR%\\" >> "%response_file%"

"%MSBUILD%" "%VCXPROJ%" @"%response_file%" 1>&2
set BUILD_EXIT=%errorlevel%

del /q "%response_file%" 2>nul

if not exist "%OUTPUT%" (
    echo OUTPUT NOT FOUND: %OUTPUT% 1>&2
    exit /b 1
)
if %BUILD_EXIT% neq 0 exit /b %BUILD_EXIT%
