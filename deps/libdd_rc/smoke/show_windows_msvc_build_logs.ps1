$ErrorActionPreference = 'Continue'
Set-StrictMode -Version 3.0

if ([Environment]::GetEnvironmentVariable('RC_X509_LIFECYCLE_SUCCEEDED', 'Process') -eq '1') {
    Write-Host 'Windows package and lifecycle probe succeeded; skipping failure diagnostics.'
    return
}

$executionRoot = [Environment]::GetEnvironmentVariable('RC_X509_BAZEL_EXECUTION_ROOT', 'Process')
if (-not $executionRoot) {
    Write-Warning 'Bazel execution root was not recorded; build-script logs are unavailable.'
    return
}

$bazelOutputRoot = Join-Path $executionRoot 'bazel-out'
$buildScriptLogs = @(Get-ChildItem -LiteralPath $bazelOutputRoot -Directory -ErrorAction Continue | ForEach-Object {
    Join-Path $_.FullName 'bin\external\rules_rs++crate+libdd_rc_crates__aws-lc-fips-sys-0.14.2\aws-lc-fips-sys_build_script'
})
$stdoutLogs = @($buildScriptLogs | ForEach-Object { "$($_).stdout.log" } | Where-Object {
    Test-Path -LiteralPath $_ -PathType Leaf
})
$stderrLogs = @($buildScriptLogs | ForEach-Object { "$($_).stderr.log" } | Where-Object {
    Test-Path -LiteralPath $_ -PathType Leaf
})

Write-Host 'AWS-LC FIPS build-script raw stdout:'
if ($stdoutLogs.Count -eq 0) {
    Write-Warning 'No AWS-LC FIPS build-script stdout logs were found.'
}
$stdoutLogs | ForEach-Object {
    Get-Content -LiteralPath $_ -ErrorAction Continue
}
Write-Host 'AWS-LC FIPS build-script raw stderr:'
if ($stderrLogs.Count -eq 0) {
    Write-Warning 'No AWS-LC FIPS build-script stderr logs were found.'
}
$stderrLogs | ForEach-Object {
    Get-Content -LiteralPath $_ -ErrorAction Continue
}
