<#
.SYNOPSIS
Builds the Datadog Agent packages for Windows. Builds everything with omnibus and packages the output into MSI, ZIP, and OCI.

.DESCRIPTION
This script builds the Datadog Agent packages for Windows, with options to configure the build environment.

.PARAMETER Flavor
Specifies the flavor of the agent. Default is the value of the environment variable AGENT_FLAVOR.

.PARAMETER BuildUpgrade
Specifies whether to build the upgrade package. Default is false.

Use this options to build an aditional MSI for testing upgrading the MSI.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.EXAMPLE
.\Build-AgentPackages.ps1 -InstallDeps $false

.EXAMPLE
.\Build-AgentPackages.ps1 -BuildOutOfSource $true -InstallDeps $true -Flavor "fips" -CheckGoVersion $true

.NOTES
This script should be run from the root of the repository.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true,
    [string] $Flavor = $env:AGENT_FLAVOR,
    [bool] $BuildUpgrade = $false
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

    if ($Flavor) {
        $inv_args += "--flavor"
        $inv_args += $Flavor
        $env:AGENT_FLAVOR=$Flavor
    }

    if ($BuildUpgrade) {
        $inv_args += "--build-upgrade"
    }

    Write-Host "dda inv -- -e winbuild.agent-package $inv_args"
    dda inv -- -e winbuild.agent-package @inv_args
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build the agent package"
        exit 1
    }

    # Show the contents of the output package directories for debugging purposes
    Get-ChildItem -Path C:\omnibus-ruby\pkg\
    Get-ChildItem -Path "C:\opt\datadog-agent\bin\agent\"
    Get-ChildItem -Path ".\omnibus\pkg\"

    if ($BuildOutOfSource) {
        # Copy the resulting package to the mnt directory
        mkdir C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID -Force -ErrorAction Stop | Out-Null
        Copy-Item -Path ".\omnibus\pkg\*" -Destination "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID" -Force -ErrorAction Stop
    }
}
