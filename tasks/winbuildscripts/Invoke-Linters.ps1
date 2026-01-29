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

trap {
    Write-Host "trap: $($_.InvocationInfo.Line.Trim()) - $_" -ForegroundColor Yellow
    continue
}

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -InstallTestingDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {

    & .\tasks\winbuildscripts\pre-go-build.ps1

    # Lint rtloader
    & dda inv -- -e rtloader.format --raise-if-changed
    $err = $LASTEXITCODE
    Write-Host Format result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "rtloader format failed $err"
        exit $err
    }

    # Lint Go
    & dda inv -- -e linter.go --debug
    $err = $LASTEXITCODE
    Write-Host Go linter result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "go linter failed $err"
        exit $err
    }

    # Lint system-probe Go
    & dda inv -- -e linter.go --build system-probe-unit-tests --targets .\pkg
    $err = $LASTEXITCODE
    Write-Host system-probe Go linter result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "system-probe go linter failed $err"
        exit $err
    }

    # Lint MSI .NET
    $timeTaken = Measure-Command {
        & dotnet format --verify-no-changes .\\tools\\windows\\DatadogAgentInstaller
        $err = $LASTEXITCODE
        Write-Host Dotnet linter result is $err
        if($err -ne 0){
            Write-Host -ForegroundColor Red "dotnet linter failed $err"
            exit $err
        }
    }
    Write-Host "Dotnet linter run time: $($timeTaken.TotalSeconds) seconds"
}
