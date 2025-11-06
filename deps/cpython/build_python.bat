:: %1: a file from sourcedir
:: %2: a file from destdir

for %%F in (%1) do set sourcedir=%%~dpF
for %%F in (%2) do set destdir=%%~dpF

:: Make path for externals absolute
:: Once in the MSBuild invocation we can't rely on relative paths
set EXTERNALS_DIR=%cd%\%EXTERNALS_DIR%

call %sourcedir%\PCbuild\build.bat -e

xcopy /y/e/s %sourcedir%PCbuild\amd64 %destdir%
