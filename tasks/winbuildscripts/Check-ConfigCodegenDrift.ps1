<#
.SYNOPSIS
Verify that regenerating the config settings code (`dda inv schema.codegen`) does
not change the Agent's runtime configuration defaults.

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

    # The task builds the Agent, dumps the config, regenerates the settings
    # code, rebuilds, dumps again, and diffs. Write the dumps to c:\mnt so they
    # are available as CI artifacts.
    & dda inv -- schema.check-codegen-drift --output-dir c:\mnt\config_codegen_drift
    $err = $LASTEXITCODE
    if ($err -ne 0) {
        Write-Host -ForegroundColor Red "Config codegen drift check failed $err"
        exit $err
    }
}
