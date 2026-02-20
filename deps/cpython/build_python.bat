:: Retrieve the directory from the given input file
for %%F in (%SRCFILE%) do set sourcedir=%%~dpF
:: Make sure input paths are proper Windows paths as needed
for %%F in (%OUTDIR%) do set destdir=%%~fF\\
for %%F in (%MSBUILD%) do set MSBUILD=%%~fF

set build_outdir=%sourcedir%\PCbuild\amd64

set script_errorlevel=0

:: Start from a clean state
%MSBUILD% "%sourcedir%\PCbuild\pcbuild.proj" /t:CleanAll
rmdir /q /s %build_outdir%
rmdir /q /s %sourcedir%\PCbuild\obj
rmdir /q /s %sourcedir%\PCbuild\win32
del /q %response_file%
del /q %sourcedir%\python.bat

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
set response_file=%sourcedir%\PCbuild\msbuild.rsp
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
:: We disable copying around of the OpenSSL libraries (as defined in openssl.props)
:: This simplifies the requirements on the input files and their names and gives us more control
echo "/p:SkipCopySSLDLL=1" >> %response_file%

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
:: --include-venv - necessary for ensurepip to work
:: --include-stable - adds python3.dll
%build_outdir%\python.exe %sourcedir%PC\layout\main.py --build %build_outdir% --precompile --copy %destdir% --include-dev --include-venv --include-stable -vv

if %errorlevel% neq 0 (
   set script_errorlevel=%errorlevel%
   goto :cleanup
)

:: Bootstrap pip
%destdir%\python.exe -m ensurepip

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
