for %%F in (%1) do set sourcedir=%%~dpF
for %%F in (%2) do set destdir=%%~dpF

call %sourcedir%\PCbuild\build.bat -e

xcopy /y/e/s %sourcedir%PCbuild\amd64 %destdir%
