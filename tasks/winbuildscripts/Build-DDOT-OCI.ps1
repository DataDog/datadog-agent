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
$cmd = "c:\mnt\tasks\winbuildscripts\Build-OmnibusTarget.ps1 -BuildOutOfSource $BuildOutOfSource -InstallDeps $InstallDeps"
if ($null -ne $CheckGoVersion) {
    $cmd += " -CheckGoVersion $CheckGoVersion"
}
powershell -C $cmd
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

# Package OCI from produced zip
powershell -NoProfile -File 'tasks\winbuildscripts\Generate-OCIPackage.ps1' `
  -package 'datadog-agent-ddot' `
  -omnibusOutput "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID" `
  -CleanupStaging

