# Run Bazel test inside the Windows Bazel container, then collect failed test logs.
#
# Keep PowerShell $ variables in this file, not in GitLab POWERSHELL_SCRIPT: GitLab
# expands $ in variables: values and breaks encoded commands (see test.yml).
param(
    [switch]$GoTestsOnly
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$excludeTargets = @(
    '-//test/new-e2e/...',
    '-//bazel/rules/dd_packaging/...',
    '-//packages/agent/linux/...'
)

if ($GoTestsOnly) {
    bazel test `
        --config=gorace `
        --config=dd-agent-go-tests-only `
        --build_tests_only `
        --keep_going `
        --remote_download_outputs=toplevel `
        --build_event_json_file=bazel-bep.json `
        //... `
        -- `
        @excludeTargets
}
else {
    bazel test `
        --config=gorace `
        --config=no-dd-agent-go-tests `
        --keep_going `
        --remote_download_outputs=toplevel `
        --build_event_json_file=bazel-bep.json `
        //... `
        -- `
        @excludeTargets
}

$testExit = $LASTEXITCODE
try {
    $projectDir = if ($env:CI_PROJECT_DIR) { $env:CI_PROJECT_DIR } else { (Get-Location).Path }
    & (Join-Path $projectDir 'tools/ci/collect-bazel-failed-testlogs.ps1') -ProjectDir $projectDir
}
catch {
    Write-Host "collect-bazel-failed-testlogs: in-container collection failed: $_"
}
exit $testExit
