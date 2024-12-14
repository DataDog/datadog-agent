<#
.SYNOPSIS
    Invoke the integration tests.

.DESCRIPTION
    Invoke the integration tests, with options to configure the build environment.

.PARAMETER BuildOutOfSource
    Indicates whether to build out of source. Default is $false.

.PARAMETER CheckGoVersion
    Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.PARAMETER InstallDeps
    Indicates whether to install dependencies. Default is $true.
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

    & .\tasks\winbuildscripts\pre-go-build.ps1

    & inv -e integration-tests
    $err = $LASTEXITCODE
    if($err -ne 0){
        Write-Host -ForegroundColor Red "test failed $err"
        [Environment]::Exit($err)
    }
    Write-Host Test passed
}