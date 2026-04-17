@echo off

:: Credential Helpers Specification: https://github.com/EngFlow/credential-helper-spec/blob/main/spec.md
set /p req=
if "%req%_" == "%req:https://buildbarn-frontend.us1.ddbuild.io=%_" (
    >&2 echo Unexpected request: "%req%"
    exit /b 2
)
if defined BUILDBARN_ID_TOKEN (
    echo {"headers":{"Authorization":["Bearer %BUILDBARN_ID_TOKEN%"]}}
) else (
    echo {}
)
