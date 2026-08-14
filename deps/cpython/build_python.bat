:: Retrieve the directory from the given input file
for %%F in (%SRCFILE%) do set sourcedir=%%~dpF
:: Make sure input paths are proper Windows paths as needed
for %%F in (%OUTDIR%) do set destdir=%%~fF\\
for %%F in (%MSBUILD%) do set MSBUILD=%%~fF
for %%F in (%PYTHON_FOR_BUILD%) do set PYTHON_FOR_BUILD=%%~fF

set build_outdir=%sourcedir%\PCbuild\amd64
set response_file=%sourcedir%\PCbuild\msbuild.rsp

set script_errorlevel=0

:: Avoid import-time bytecode writes during the build and ensurepip bootstrap.
set PYTHONDONTWRITEBYTECODE=1

:: Point MSBuild at the hermetic MSVC, Windows SDK and VC project system from
:: @toolchains_msvc. Each root is recovered from a marker file staged inside it.
for %%F in (%VCTOOLS_VERSION_FILE%) do set "MSVC_ROOT=%%~dpF"
for %%F in (%WINSDK_MARKER%) do set "WINSDK_ROOT=%%~dpF"
for %%F in (%VCTARGETS_MARKER%) do set "VCTARGETS_PATH=%%~dpF"
set /p VCTOOLS_VERSION=<%VCTOOLS_VERSION_FILE%

:: Microsoft.Cpp.VCTools.props reads toolset-suffixed properties (VCInstallDir_170
:: for VC/v170), so derive the suffix from the toolset directory name.
for %%F in ("%VCTARGETS_PATH:~0,-1%") do set "VC_TOOLSET_DIR=%%~nxF"
set "VC_TOOLSET=_%VC_TOOLSET_DIR:~1%"

:: The SDK ships exactly one versioned include directory.
for /d %%D in ("%WINSDK_ROOT%Include\10.0.*") do set "WINSDK_PLATFORM_VERSION=%%~nxD"

:: Make paths for external deps absolute
:: Once in the MSBuild invocation we can't rely on relative paths
:: Note that these need to finish in a backslash separator specifically,
:: as many parts of the build setup depend on this.
set BZ2_DIR=%cd%\%BZ2_DIR%\\
set MPDECIMAL_DIR=%cd%\%MPDECIMAL_DIR%\\
set SQLITE3_DIR=%cd%\%SQLITE3_DIR%\\
set XZ_DIR=%cd%\%XZ_DIR%\\
set ZLIB_DIR=%cd%\%ZLIB_DIR%\\
set LIBFFI_DIR=%cd%\%LIBFFI_DIR%\\
set OPENSSL_DIR=%cd%\%OPENSSL_DIR%\\
set TCLTK_DIR=%cd%\%TCLTK_DIR%\\

:: Properties pointing at external dependencies directories are taken from PCbuild/python.props
:: Note that the build.bat script only accepts 9 extra arguments that can be passed through to MSBuild,
:: so we write the arguments to a .rsp file that will be added to msbuild calls instead to not have to worry
:: about that
echo "" > %response_file%
echo "/p:bz2Dir=%BZ2_DIR%" >> %response_file%
echo "/p:mpdecimalDir=%MPDECIMAL_DIR%" >> %response_file%
echo "/p:sqlite3Dir=%SQLITE3_DIR%" >> %response_file%
echo "/p:lzmaDir=%XZ_DIR%" >> %response_file%
echo "/p:zlibDir=%ZLIB_DIR%" >> %response_file%
echo "/p:libffiDir=%LIBFFI_DIR%" >> %response_file%
echo "/p:opensslOutDir=%OPENSSL_DIR%" >> %response_file%
echo "/p:tcltkdir=%TCLTK_DIR%" >> %response_file%
echo "/p:TclVersion=%TCL_VERSION%" >> %response_file%
:: Without DisableRegistryUse, the VC props fall back to registry lookups and a
:: host Visual Studio silently wins over everything set below.
echo "/p:DisableRegistryUse=true" >> %response_file%
:: The VC props expect these roots to keep their trailing separator, and MSBuild reads
:: \" in a response file as an escaped quote, so the final backslash has to be doubled.
echo "/p:VCTargetsPath=%VCTARGETS_PATH%\" >> %response_file%
echo "/p:VCInstallDir%VC_TOOLSET%=%MSVC_ROOT%\" >> %response_file%
echo "/p:VCToolsInstallDir%VC_TOOLSET%=%MSVC_ROOT%Tools\\" >> %response_file%
echo "/p:VCToolsRedistInstallDir%VC_TOOLSET%=%MSVC_ROOT%Redist\\" >> %response_file%
echo "/p:VCToolsVersion=%VCTOOLS_VERSION%" >> %response_file%
:: CPython's FindVCRedistDir target expects the Visual Studio layout (Redist\MSVC\<version>\),
:: while the staged tree flattens it the same way Tools\ is flattened. The target appends the
:: platform directory itself.
echo "/p:VCRedistDir=%MSVC_ROOT%Redist\\" >> %response_file%
echo "/p:WindowsSdkDir_10=%WINSDK_ROOT%\" >> %response_file%
echo "/p:UniversalCRTSdkDir_10=%WINSDK_ROOT%\" >> %response_file%
echo "/p:WindowsTargetPlatformVersion=%WINSDK_PLATFORM_VERSION%" >> %response_file%
:: With DisableRegistryUse the VC props only derive WindowsSdkDir for the 8.1 fallback, and
:: the SDK's own uCRT.props resolves UCRTContentRoot from the registry either way.
echo "/p:WindowsSdkDir=%WINSDK_ROOT%\" >> %response_file%
echo "/p:UCRTContentRoot=%WINSDK_ROOT%\" >> %response_file%
:: The SDK's DesignTime UAP.props, which normally contributes the headers, libs and tools
:: below, is not part of the staged tree. Semicolons must be escaped as %%3B, otherwise
:: MSBuild reads them as switch separators in the response file.
set "WINSDK_INC=%WINSDK_ROOT%Include\%WINSDK_PLATFORM_VERSION%"
echo "/p:WindowsSDK_IncludePath=%WINSDK_INC%\um%%3B%WINSDK_INC%\shared%%3B%WINSDK_INC%\winrt%%3B%WINSDK_INC%\cppwinrt" >> %response_file%
echo "/p:WindowsSDK_LibraryPath_x64=%WINSDK_ROOT%Lib\%WINSDK_PLATFORM_VERSION%\um\x64" >> %response_file%
echo "/p:WindowsSDK_ExecutablePath_x64=%WINSDK_ROOT%bin\%WINSDK_PLATFORM_VERSION%\x64" >> %response_file%
:: Only x64-hosted tools are staged, and pcbuild.proj builds the freeze tool for this arch.
echo "/p:PreferredToolArchitecture=x64" >> %response_file%
:: We disable copying around of the OpenSSL libraries (as defined in openssl.props)
:: This simplifies the requirements on the input files and their names and gives us more control
echo "/p:SkipCopySSLDLL=1" >> %response_file%
:: but _hashlib.pyd needs OPENSSL_DIR registered as a DLL search directory for PGO tests.
echo import os; os.add_dll_directory(r'%OPENSSL_DIR%') >sitecustomize.py
set "PYTHONPATH=%cd%;%PYTHONPATH%"

:: Start from a clean state. This runs after the response file is complete: with no
:: host Visual Studio to fall back on, even CleanAll needs VCTargetsPath.
%MSBUILD% "%sourcedir%\PCbuild\pcbuild.proj" /t:CleanAll
rmdir /q /s %build_outdir%
rmdir /q /s %sourcedir%\PCbuild\obj
rmdir /q /s %sourcedir%\PCbuild\win32
del /q %sourcedir%\python.bat

:: -e flag would normally also fetch external dependencies, but we have a patch inhibiting that;
:: the flag is still needed because otherwise modules depending on some of those external dependencies
:: won't be built.
call %sourcedir%\PCbuild\build.bat -e --pgo

if %errorlevel% neq 0 (
   set script_errorlevel=%errorlevel%
   goto :cleanup
)

@echo on

:: Needed to avoid xcopy from failing when copying files out of this dir
for %%F in (%OPENSSL_DIR%) do set OPENSSL_DIR=%%~fF

:: Copy OpenSSL files to where the layout script expects them.
:: The Python build would do this itself when SkipCopySSLDLL is not set,
:: since we enabled that, we need to now copy them manually
xcopy /f %OPENSSL_DIR%*.lib %build_outdir%\
xcopy /f %OPENSSL_DIR%*.dll %build_outdir%\

:: Create final layout from the build
:: --include-dev - include include/ and libs/ directories
:: --include-venv - include venv support
:: --include-stable - adds python3.dll
%build_outdir%\python.exe %sourcedir%PC\layout\main.py --build %build_outdir% --copy %destdir% --include-dev --include-venv --include-stable -vv

if %errorlevel% neq 0 (
   set script_errorlevel=%errorlevel%
   goto :cleanup
)

:cleanup
:: Clean so that no artifacts produced by the build remain
%MSBUILD% "%sourcedir%\PCbuild\pcbuild.proj" /t:CleanAll
rmdir /q /s %build_outdir%
rmdir /q /s %sourcedir%\PCbuild\obj
rmdir /q /s %sourcedir%\PCbuild\win32
del /q %response_file%
del /q %sourcedir%\python.bat

if %script_errorlevel% neq 0 (
   exit /b %script_errorlevel%
)
