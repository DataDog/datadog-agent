<#
.SYNOPSIS
Builds the Datadog Agent MSI from the payload produced by Build-AgentPackages.ps1 -BuildMsi 0.
#>
param(
    [bool] $BuildOutOfSource = $false,
    [bool] $InstallDeps = $true,
    [bool] $BuildUpgrade = $false
)

. "$PSScriptRoot\common.ps1"

trap {
    Write-Host "trap: $($_.InvocationInfo.Line.Trim()) - $_" -ForegroundColor Yellow
    continue
}

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $false `
    -CheckGoVersion $false `
    -Command {
    if ($InstallDeps) {
        Install-Deps
    }

    $inv_args = @()

    if ($BuildUpgrade) {
        $inv_args += "--build-upgrade"
    }

    Write-Host "dda inv -- -e winbuild.agent-msi $inv_args"
    dda inv -- -e winbuild.agent-msi @inv_args
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build the agent MSI"
        exit 1
    }

    Get-ChildItem -Path ".\omnibus\pkg\"

    if ($BuildOutOfSource) {
        mkdir C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID -Force -ErrorAction Stop | Out-Null
        Copy-Item -Path ".\omnibus\pkg\*.msi" -Destination "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID" -Force -ErrorAction Stop
    }
}
