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

$WINSIGN_VERSION = "0.3.1"

Write-Host "Installing Windows Codesign Helper $WINSIGN_VERSION"
# $ErrorActionPreference = 'Stop'
# $ProgressPreference = 'SilentlyContinue'

Write-Host -ForegroundColor Green "Installing Windows Codesign Helper $WINSIGN_VERSION"

## need to have more rigorous download at some point, but
$codesign_base = "windows_code_signer-$($WINSIGN_VERSION)-py3-none-any.whl"
$codesign_wheel = "https://s3.amazonaws.com/dd-agent-omnibus/windows-code-signer/$($codesign_base)"
# $codesign_wheel = "https://s3.amazonaws.com/dd-ci-persistent-artefacts-build-stable/datadog-agent/celian-windows-code-signer/$($codesign_base)"
$codesign_wheel_target = "c:\devtools\$($codesign_base)"
(New-Object System.Net.WebClient).DownloadFile($codesign_wheel, $codesign_wheel_target)

# TODO: Get-RemoteFile -RemoteFile $codesign_wheel -LocalFile $codesign_wheel_target -VerifyHash $WINSIGN_SHA256

python -m pip install $codesign_wheel_target
If ($lastExitCode -ne "0") { throw "Previous command returned $lastExitCode" }


Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {
    $inv_args = @(
        "--skip-deps"
    )




    # Write-Host "dda inv -- -e winbuild.test-boto $inv_args"
    # dda inv -- -e winbuild.test-boto
    # if ($LASTEXITCODE -ne 0) {
    #     Write-Error "Failed to test boto"
    #     exit 1
    # }

    # exit 1
















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
