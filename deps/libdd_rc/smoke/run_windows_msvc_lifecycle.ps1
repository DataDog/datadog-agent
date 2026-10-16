$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

[Environment]::SetEnvironmentVariable('RC_X509_LIFECYCLE_SUCCEEDED', $null, 'Process')

$PlatformArgs = @(
    '-c', 'dbg',
    '--platforms=//bazel/platforms:windows_x86_64',
    '--extra_toolchains=@rust_toolchains//:rw-395021384',
    '--extra_toolchains=@winlibs_mingw64//:mingw_cc_toolchain',
    '--action_env=PATH',
    '--@rules_rust//cargo/settings:debug_std_streams_output_group=true'
)
$PackageTarget = '//packages/agent/dependencies:rc_x509_packaged_agent_windows_archive'

$executionRoots = @(bazel info @PlatformArgs execution_root)
if ($LASTEXITCODE -ne 0 -or $executionRoots.Count -ne 1) {
    throw "Could not resolve one Bazel execution root: $executionRoots"
}
$executionRoot = $executionRoots[0]
$env:RC_X509_BAZEL_EXECUTION_ROOT = $executionRoot
Write-Host "Bazel execution root: $executionRoot"

function Resolve-BazelOutputPath {
    param([Parameter(Mandatory = $true)][string]$OutputPath)

    if ([IO.Path]::IsPathRooted($OutputPath)) {
        return $OutputPath
    }
    return (Join-Path $executionRoot $OutputPath)
}

bazel build @PlatformArgs --jobs=8 --verbose_failures --remote_download_outputs=all $PackageTarget
if ($LASTEXITCODE -ne 0) {
    throw "Bazel build failed with exit code $LASTEXITCODE"
}

$packageOutputs = @(bazel cquery @PlatformArgs --output=files $PackageTarget)
if ($LASTEXITCODE -ne 0) {
    throw "Windows package archive cquery failed with exit code $LASTEXITCODE"
}
Write-Host 'Windows package archive declared outputs:'
$packageOutputs | ForEach-Object { Write-Host "  $_" }
$packageArchives = @($packageOutputs | Where-Object { $_ -match '\.tar$' })
if ($packageArchives.Count -ne 1) {
    throw "Expected one Windows package archive, found $($packageArchives.Count): $packageOutputs"
}
$packageArchivePath = Resolve-BazelOutputPath ($packageArchives[0])
if (-not (Test-Path -LiteralPath $packageArchivePath)) {
    throw "Windows package archive does not exist: $packageArchivePath"
}

$stageDir = Join-Path $env:TEMP "rc-x509-msvc-lifecycle-$env:CI_JOB_ID"
if (Test-Path -LiteralPath $stageDir) {
    Remove-Item -LiteralPath $stageDir -Recurse -Force
}
New-Item -Path $stageDir -ItemType Directory | Out-Null
& tar.exe -xf $packageArchivePath -C $stageDir
if ($LASTEXITCODE -ne 0) {
    throw "Could not extract Windows package archive; tar.exe exited with code $LASTEXITCODE"
}

$agentBinDir = Join-Path $stageDir 'bin\agent'
$expectedFiles = @(
    (Join-Path $agentBinDir 'agent.exe'),
    (Join-Path $agentBinDir 'windows_msvc_lifecycle.exe'),
    (Join-Path $agentBinDir 'rc_x509_ffi.dll'),
    (Join-Path $agentBinDir 'aws_lc_fips_0_14_2_crypto.dll')
)
$missingFiles = @($expectedFiles | Where-Object { -not (Test-Path -LiteralPath $_ -PathType Leaf) })
if ($missingFiles.Count -ne 0) {
    throw "Windows package archive is missing expected files: $missingFiles"
}

Write-Host 'Packaged Windows lifecycle probe:'
Get-ChildItem -LiteralPath $agentBinDir | Format-Table -AutoSize
& (Join-Path $agentBinDir 'windows_msvc_lifecycle.exe')
if ($LASTEXITCODE -ne 0) {
    throw "Packaged Windows MSVC/MinGW lifecycle probe exited with code $LASTEXITCODE"
}

Write-Host 'Packaged Windows Agent version probe:'
[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_ENABLED', 'false', 'Process')
[Environment]::SetEnvironmentVariable('DD_REMOTE_CONFIGURATION_X509_ENABLED', 'false', 'Process')
& (Join-Path $agentBinDir 'agent.exe') version
if ($LASTEXITCODE -ne 0) {
    throw "Packaged Windows Agent version probe exited with code $LASTEXITCODE"
}
[Environment]::SetEnvironmentVariable('RC_X509_LIFECYCLE_SUCCEEDED', '1', 'Process')
