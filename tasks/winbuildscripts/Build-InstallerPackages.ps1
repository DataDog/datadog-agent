<#
.SYNOPSIS
Builds the Datadog Installer packages for Windows. Builds everything with omnibus and packages the output into MSI, ZIP, and OCI.

.DESCRIPTION
This script builds the Datadog Installer packages for Windows, with options to configure the build environment.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.EXAMPLE
.\Build-InstallerPackages.ps1 -InstallDeps $false

.EXAMPLE
.\Build-InstallerPackages.ps1 -BuildOutOfSource $true -InstallDeps $true -CheckGoVersion $true

.NOTES
This script should be run from the root of the repository.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true
)

. "$PSScriptRoot\common.ps1"

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {
    $inv_args = @(
        "--skip-deps"
    )

    Write-Host "dda inv -- -e winbuild.installer-package $inv_args"
    dda inv -- -e winbuild.installer-package @inv_args
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build the agent package"
        exit 1
    }

    # Show the contents of the output package directories for debugging purposes
    Get-ChildItem -Path C:\omnibus-ruby\pkg\
    Get-ChildItem -Path C:\opt\datadog-installer
    Get-ChildItem -Path ".\omnibus\pkg\"

    if ($BuildOutOfSource) {
        # Copy the resulting package to the mnt directory
        mkdir C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID -Force -ErrorAction Stop | Out-Null
        Copy-Item -Path ".\omnibus\pkg\*" -Destination "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID" -Force -ErrorAction Stop
    }
}
