# Collect Bazel test.log / test.xml for failed tests into CI artifacts.
#
# Windows Bazel jobs run inside Docker with outputBase C:\bob bind-mounted to
# $CI_PROJECT_DIR\.cache\bob on the host. This script resolves test output URIs
# from bazel-bep.json and copies logs without parsing test stdout (Bazel often
# truncates console output before the failure summary).
param(
    [string]$ProjectDir = $env:CI_PROJECT_DIR,
    [string]$BepFile = 'bazel-bep.json',
    [string]$DestDir = 'bazel-failed-testlogs'
)

$ErrorActionPreference = 'Stop'

if (-not $ProjectDir) {
    $ProjectDir = (Get-Location).Path
    Write-Host "collect-bazel-failed-testlogs: CI_PROJECT_DIR unset; using $ProjectDir"
}

$dest = Join-Path $ProjectDir $DestDir
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$bepPath = Join-Path $ProjectDir $BepFile
if (-not (Test-Path $bepPath)) {
    Write-Host "collect-bazel-failed-testlogs: no BEP file at $bepPath"
    exit 0
}

# Inside the Bazel container output lives under C:\bob; on the host job slot it
# is bind-mounted under .cache\bob (see tools/ci/docker-run-with-bazel-cache.ps1).
$bobRoots = @(
    'C:\bob',
    (Join-Path $ProjectDir '.cache\bob')
) | Where-Object { Test-Path $_ } | Select-Object -Unique

if (-not $bobRoots) {
    Write-Host 'collect-bazel-failed-testlogs: no Bazel output base found (C:\bob or .cache\bob)'
    exit 0
}

function Resolve-BazelTestOutputPath {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string[]]$Roots
    )

    $rel = $Uri -replace '^file:///C:/bob/', '' -replace '^file://C:/bob/', '' -replace '/', '\'
    foreach ($root in $Roots) {
        $candidate = Join-Path $root $rel
        if (Test-Path $candidate) {
            return $candidate
        }
    }
    return $null
}

function Sanitize-ArtifactName {
    param([Parameter(Mandatory = $true)][string]$Label)

    ($Label -replace '^//', '' -replace ':', '__' -replace '/', '__')
}

$collected = 0
Get-Content -Path $bepPath | ForEach-Object {
    if ($_ -notmatch '"testResult"') {
        return
    }

    try {
        $ev = $_ | ConvertFrom-Json
    }
    catch {
        return
    }

    if (-not $ev.testResult -or $ev.testResult.status -ne 'FAILED') {
        return
    }

    $label = $ev.id.testResult.label
    if (-not $label) {
        Write-Host 'collect-bazel-failed-testlogs: FAILED testResult without label; skipping line'
        return
    }

    $safeLabel = Sanitize-ArtifactName $label
    foreach ($out in @($ev.testResult.testActionOutput)) {
        if ($out.name -notin @('test.log', 'test.xml') -or -not $out.uri) {
            continue
        }

        $src = Resolve-BazelTestOutputPath -Uri $out.uri -Roots $bobRoots
        if (-not $src) {
            Write-Host "collect-bazel-failed-testlogs: missing $($out.name) for $label uri=$($out.uri)"
            continue
        }

        $outName = "${safeLabel}__$($out.name)"
        $target = Join-Path $dest $outName
        try {
            Copy-Item -Path $src -Destination $target -Force -ErrorAction Stop
            Write-Host "collect-bazel-failed-testlogs: copied $outName from $src ($((Get-Item $src).Length) bytes)"
            $collected++
        }
        catch {
            Write-Host "collect-bazel-failed-testlogs: failed to copy $outName from $src: $_"
        }
    }
}

Write-Host "collect-bazel-failed-testlogs: collected $collected file(s) under $dest"
