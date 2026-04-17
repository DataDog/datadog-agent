@echo off

:: Credential Helpers Specification: https://github.com/EngFlow/credential-helper-spec/blob/main/spec.md
set /p _=
if defined BUILDBARN_ID_TOKEN (
    echo {"headers":{"Authorization":["Bearer %BUILDBARN_ID_TOKEN%"]}}
) else (
    echo {}
)
