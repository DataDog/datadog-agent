$ErrorActionPreference = 'Continue'
Set-StrictMode -Version 3.0

if (-not $env:RC_X509_BAZEL_EXECUTION_ROOT) {
    Write-Warning 'Bazel execution root was not recorded; build-script logs are unavailable.'
    return
}

$bazelOutputRoot = Join-Path $env:RC_X509_BAZEL_EXECUTION_ROOT 'bazel-out'
$buildScriptLogs = @(Get-ChildItem -LiteralPath $bazelOutputRoot -Directory -ErrorAction Continue | ForEach-Object {
    Join-Path $_.FullName 'bin\external\rules_rs++crate+libdd_rc_crates__aws-lc-fips-sys-0.14.2\aws-lc-fips-sys_build_script'
})

Write-Host 'AWS-LC FIPS build-script raw stdout:'
$buildScriptLogs | ForEach-Object {
    Get-Content -LiteralPath "$($_).stdout.log" -ErrorAction Continue
}
Write-Host 'AWS-LC FIPS build-script raw stderr:'
$buildScriptLogs | ForEach-Object {
    Get-Content -LiteralPath "$($_).stderr.log" -ErrorAction Continue
}
