<#
.SYNOPSIS
Invoke Bazel coverage on Windows.

.DESCRIPTION
Runs Bazel coverage for Windows Go tests, then performs CI post-processing:
collect Bazel test results, upload JUnit data, collect the coverage profile, and upload coverage.

.PARAMETER BepFile
Path to the Bazel build event JSON file produced by --build_event_json_file.

.PARAMETER ResultJson
Path where test result JSON should be written.

.PARAMETER JunitTar
Path where the JUnit tarball should be written.

.PARAMETER CoverageOut
Path where the Bazel coverage profile should be copied for artifacts and upload.

Requires the AGENT_API_KEY_ORG2 environment variable to be set for JUnit and coverage uploads.
#>
param(
    [Parameter(Mandatory = $true)]
    [string] $BepFile,

    [Parameter(Mandatory = $true)]
    [string] $ResultJson,

    [Parameter(Mandatory = $true)]
    [string] $JunitTar,

    [Parameter(Mandatory = $true)]
    [string] $CoverageOut
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3.0

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path

function Invoke-NonFatalStep {
    param(
        [Parameter(Mandatory = $true)]
        [string] $Name,

        [Parameter(Mandatory = $true)]
        [scriptblock] $ScriptBlock
    )

    try {
        Write-Host "Starting ${Name}"
        $global:LASTEXITCODE = 0
        & $ScriptBlock
        if ($LASTEXITCODE -ne 0) {
            throw "${Name} failed with exit code $LASTEXITCODE"
        }
    }
    catch {
        Write-Host -ForegroundColor Red "${Name} failed (non-fatal): $($_.Exception.Message)"
    }
}

$bazelExitCode = 0
$global:LASTEXITCODE = 0
try {
    & bazel coverage --config=go --config=gorace --config=dd-agent-go-tests-only --build_tests_only --keep_going --remote_download_outputs=toplevel --build_event_json_file=$BepFile //...
    if ($LASTEXITCODE -ne 0) {
        $bazelExitCode = $LASTEXITCODE
        Write-Host -ForegroundColor Red "bazel coverage failed with exit code $bazelExitCode"
    }
}
catch {
    $bazelExitCode = if ($LASTEXITCODE -ne 0) { $LASTEXITCODE } else { 1 }
    Write-Host -ForegroundColor Red "bazel coverage failed (exit $bazelExitCode): $($_.Exception.Message)"
}

# Logic that tries to emulate gitlab `after_script` (always run and don't fail the job on failure)

$datadogApiKey = ''
Invoke-NonFatalStep -Name 'Datadog API key fetch' -ScriptBlock {
    if ([string]::IsNullOrEmpty($Env:AGENT_API_KEY_ORG2)) {
        throw 'AGENT_API_KEY_ORG2 is empty'
    }

    . (Join-Path $RepoRoot 'tasks/winbuildscripts/common.ps1')
    # Retrieve API key from vault. junit and coverage uploader expect different environment variable names for this.
    $script:datadogApiKey = Get-VaultSecret -parameterName $Env:AGENT_API_KEY_ORG2 -parameterField token
    $Env:DATADOG_API_KEY = $script:datadogApiKey
    $Env:DD_API_KEY = $script:datadogApiKey
}

Invoke-NonFatalStep -Name 'Bazel test result processing and JUnit upload' -ScriptBlock {
    & dda inv -- -e bazel.process-test-results --bep-file=$BepFile --junit-tar=$JunitTar --result-json=$ResultJson
    if ($LASTEXITCODE -ne 0) {
        throw "bazel.process-test-results failed with exit code $LASTEXITCODE"
    }

    & dda inv -- -e junit-upload --tgz-path $JunitTar --result-json=$ResultJson
}

Invoke-NonFatalStep -Name 'Coverage artifact collection and upload' -ScriptBlock {
    $bazelCoverage = "$(bazel info output_path)/_coverage/_coverage_report.dat"
    if ($LASTEXITCODE -ne 0) {
        throw "bazel info output_path failed with exit code $LASTEXITCODE"
    }

    Copy-Item $bazelCoverage $CoverageOut
    & dda inv -- -e coverage.upload-to-datadog --coverage-file $CoverageOut
}

# Propagate bazel exit code
if ($bazelExitCode -ne 0) {
    exit $bazelExitCode
}
