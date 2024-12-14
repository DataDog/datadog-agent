<#
.SYNOPSIS
    Invoke the linters.

.DESCRIPTION
    Invoke the linters, with options to configure the build environment.

    Runs linters for rtloader, Go, and MSI .NET.

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

    # Lint rtloader
    & inv -e rtloader.format --raise-if-changed
    $err = $LASTEXITCODE
    Write-Host Format result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "rtloader format failed $err"
        exit 1
    }

    # Lint Go
    & inv -e linter.go --debug
    $err = $LASTEXITCODE
    Write-Host Go linter result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "go linter failed $err"
        exit 1
    }

    # Lint MSI .NET
    $timeTaken = Measure-Command {
        & dotnet format --verify-no-changes .\\tools\\windows\\DatadogAgentInstaller
        $err = $LASTEXITCODE
        Write-Host Dotnet linter result is $err
        if($err -ne 0){
            Write-Host -ForegroundColor Red "dotnet linter failed $err"
            exit 1
        }
    }
    Write-Host "Dotnet linter run time: $($timeTaken.TotalSeconds) seconds"
}