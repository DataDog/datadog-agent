<#
.SYNOPSIS
Invoke the linters.

.DESCRIPTION
Invoke the linters, with options to configure the build environment.

Runs linters for rtloader, Go, and MSI .NET.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

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
    -InstallTestingDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {

    & pip install git+https://github.com/DataDog/datadog-agent-dev.git@kfairise/support-feature-flag-ci
    Write-Host "Invoking dda"
    & dda info owners code .gitlab-ci.yaml
    & dda bzl
}
