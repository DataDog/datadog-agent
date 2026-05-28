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

trap {
    Write-Host "trap: $($_.InvocationInfo.Line.Trim()) - $_" -ForegroundColor Yellow
    continue
}

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

    # Generate the static quality gate inventory report for the MSI: wire size
    # is the .msi file size, on-disk size + inventory come from the companion .zip.
    if ($env:STATIC_QUALITY_GATE_NAME) {
        $pkgDir = "C:\mnt\omnibus\pkg\pipeline-$env:CI_PIPELINE_ID"
        $msis = @(Get-ChildItem -Path "$pkgDir\datadog-agent-7*x86_64.msi")
        $zips = @(Get-ChildItem -Path "$pkgDir\datadog-agent-7*x86_64.zip")
        if ($msis.Count -ne 1) {
            Write-Error "Expected exactly 1 MSI file matching 'datadog-agent-7*x86_64.msi', got $($msis.Count)"
            exit 1
        }
        if ($zips.Count -ne 1) {
            Write-Error "Expected exactly 1 ZIP file matching 'datadog-agent-7*x86_64.zip', got $($zips.Count)"
            exit 1
        }
        $reportPrefix = $env:STATIC_QUALITY_GATE_NAME -replace '^static_quality_gate_', ''
        $reportPath = Join-Path $pkgDir "${reportPrefix}_size_report_${env:CI_COMMIT_SHORT_SHA}.yml"
        Write-Host "Measuring MSI '$($msis[0].Name)' with inventory from '$($zips[0].Name)'"
        dda inv -- -e quality-gates.measure-package-local `
            --package-path $zips[0].FullName `
            --wire-size-source $msis[0].FullName `
            --gate-name $env:STATIC_QUALITY_GATE_NAME `
            --build-job-name $env:CI_JOB_NAME `
            --output-path $reportPath `
            --debug
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Static quality gate measurement failed"
            exit 1
        }

        # The SQG runner reads reports exclusively from S3 (it no longer
        # downloads build artifacts), so the report has to be uploaded.
        # Skip uploads from cross-repo child pipelines so they don't overwrite
        # the per-commit report from this repo's own pipeline.
        if ($env:CI_PIPELINE_SOURCE -eq "pipeline" -or $env:CI_PIPELINE_SOURCE -eq "parent_pipeline") {
            Write-Host "Pipeline source is '$env:CI_PIPELINE_SOURCE'; skipping report upload"
        } else {
            $bucketBasePath = "s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/GATE_REPORTS/$env:CI_COMMIT_SHA"
            $reportFilename = Split-Path $reportPath -Leaf
            Write-Host "Uploading report to $bucketBasePath/$reportFilename"
            # Explicit `.exe` because Ruby's aws-sdk-core ships an `aws.rb`
            # shim that shadows the real CLI on the Windows build container.
            aws.exe s3 cp --only-show-errors --region us-east-1 --sse AES256 `
                $reportPath `
                "$bucketBasePath/$reportFilename"
            if ($LASTEXITCODE -ne 0) {
                Write-Error "Static quality gate report upload failed"
                exit 1
            }
        }
    }
}
