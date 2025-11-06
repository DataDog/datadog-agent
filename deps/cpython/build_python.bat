:: %1: a file from sourcedir
:: %2: a file from destdir

for %%F in (%1) do set sourcedir=%%~dpF
for %%F in (%2) do set destdir=%%~dpF

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
:: if we ever go over that limit, we'd neet to find a different mechanism to set these.
call %sourcedir%\PCbuild\build.bat -e^
  "/p:bz2Dir=%BZ2_DIR%"^
  "/p:mpdecimalDir=%MPDECIMAL_DIR%"^
  "/p:sqlite3Dir=%SQLITE3_DIR%"^
  "/p:lzmaDir=%XZ_DIR%"^
  "/p:zlibDir=%ZLIB_DIR%"^
  "/p:libffiDir=%LIBFFI_DIR%"^
  "/p:opensslOutDir=%OPENSSL_DIR%"^
  "/p:tcltkdir=%TCLTK_DIR%"^
  "/p:TclVersion=%TCL_VERSION%"

if ERRORLEVEL 1 exit /b %ERRORLEVEL%

xcopy /y/e/s %sourcedir%PCbuild\amd64 %destdir%

