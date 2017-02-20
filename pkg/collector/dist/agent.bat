@echo off
set DIRNAME=%~dp0%
set PYTHONHOME=%DIRNAME%\dist\python
set PATH=%PYTHONHOME%;%PATH%

%DIRNAME%\agent.exe  %*