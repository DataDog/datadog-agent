<#
.SYNOPSIS
Generate the config schema artifacts.

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
    -CheckGoVersion $CheckGoVersion `
    -Command {

    & dda inv -- -e agent.build
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Agent build failed $err"
        exit $err
    }

    & dda inv -- schema.generate
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Schema generation failed $err"
        exit $err
    }
}
