[CmdletBinding()]
param(
    [bool] $BuildOutOfSource = $true,
    [bool] $InstallDeps = $true,
    [nullable[bool]] $CheckGoVersion
)

$ErrorActionPreference = 'Stop'

Set-Location 'C:\mnt'
. .\tasks\winbuildscripts\common.ps1

Enable-DevEnv

# Build Omnibus ddot target
$omniScript = "c:\mnt\tasks\winbuildscripts\Build-OmnibusTarget.ps1"
if ($null -ne $CheckGoVersion) {
    & $omniScript -BuildOutOfSource:$BuildOutOfSource -InstallDeps:$InstallDeps -CheckGoVersion:$CheckGoVersion
} else {
    & $omniScript -BuildOutOfSource:$BuildOutOfSource -InstallDeps:$InstallDeps
}
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

# Package OCI from produced zip
powershell -NoProfile -File 'tasks\winbuildscripts\Generate-OCIPackage.ps1' `
  -package 'datadog-agent-ddot' `
  -omnibusOutput "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID" `
  -CleanupStaging

# Propagate failure from packaging
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

