@echo off

@echo =======================================================
@echo Stackstate modded....
@echo =======================================================
@echo.
@echo Agent Windows Build Docker Container

@echo AWS_NETWORKING is %AWS_NETWORKING%
if defined AWS_NETWORKING (
    @echo Detected AWS container, setting up networking
    powershell -C "c:\aws_networking.ps1"
)
%*
exit /b %ERRORLEVEL%
