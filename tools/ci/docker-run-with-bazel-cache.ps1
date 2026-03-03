$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

mkdir "$env:XDG_CACHE_HOME" -Force | Out-Null
& docker run --rm --env=BAZELISK_HOME --env=CI --env=XDG_CACHE_HOME --volume="${env:XDG_CACHE_HOME}:${env:XDG_CACHE_HOME}" $args
exit $LASTEXITCODE
