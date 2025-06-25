<#
.SYNOPSIS
Builds an Omnibus project for Windows.

.DESCRIPTION
This script builds an Omnibus Project for Windows, with options to configure the build environment.

.PARAMETER Flavor
Specifies the flavor of the agent. Default is the value of the environment variable AGENT_FLAVOR.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.PARAMETER TargetProject
Specifies the target project for the build. This parameter is mandatory and defaults to the value of the environment variable OMNIBUS_TARGET.

.EXAMPLE
.\Build-OmnibusTarget.ps1 -InstallDeps $false

.EXAMPLE
.\Build-OmnibusTarget.ps1 -BuildOutOfSource $true -InstallDeps $true -Flavor "fips" -CheckGoVersion $true

.NOTES
This script should be run from the root of the repository.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true,
    [string] $TargetProject = $env:OMNIBUS_TARGET
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

    if ($TargetProject) {
        $inv_args += "--target-project"
        $inv_args += $TargetProject
        $env:OMNIBUS_TARGET=$TargetProject
    } else {
        Write-Error "Target project is required"
        Write-Error "To build the (default) Agent package, use Build-AgentPackages.ps1 instead"
        exit 1
    }

    Write-Host "dda inv -- -e omnibus.build $inv_args"
    dda inv -- -e omnibus.build @inv_args
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
