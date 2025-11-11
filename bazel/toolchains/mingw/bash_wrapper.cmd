@echo off
setlocal
REM Wrapper to start bash with PATH converted from Windows-style (;) to Unix-style (:)

REM Replace semicolons with colons in PATH using batch string substitution
set "PATH=%PATH:;=:%"

REM Ensure MSYS tools are in PATH
set "PATH=/usr/bin:/mingw64/bin:%PATH%"

REM Pass all arguments to bash
C:\tools\msys64\usr\bin\bash.exe %*
endlocal
