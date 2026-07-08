@echo off
setlocal

:: Credential Helpers Specification: https://github.com/EngFlow/credential-helper-spec/blob/main/spec.md
set /p _=

:: Preset token: always honor it
if defined BUILDBARN_ID_TOKEN (
  call :write_auth_headers "%BUILDBARN_ID_TOKEN%"
  exit /b 0
)

:: CI: never attempt interactive Vault login, but let `bazel` honor any applicable --remote_local_fallback* options
if defined CI (
  echo {}
  exit /b 0
)

:: Dev: mint a Vault OIDC token, logging in if needed, cached by `bazel` as per --credential_helper_cache_duration
if not defined VAULT_ADDR set "VAULT_ADDR=https://vault.us1.ddbuild.io"
call :fetch_token
if defined token (
  call :write_auth_headers "%token%"
  exit /b 0
)
>&2 vault login -address="%VAULT_ADDR%" -method=oidc || exit /b 1
call :fetch_token
if defined token (
  call :write_auth_headers "%token%"
  exit /b 0
)
>&2 echo Vault returned an empty token!
exit /b 1

:fetch_token
set "token="
for /f "usebackq delims=" %%t in (`vault read -address "%VAULT_ADDR%" -field token identity/oidc/token/buildbarn`) do set "token=%%t"
goto :eof

:write_auth_headers
echo {"headers":{"Authorization":["Bearer %~1"]}}
goto :eof
