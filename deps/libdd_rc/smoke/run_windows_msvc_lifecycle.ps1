$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version 3.0

$PlatformArgs = @(
    '-c', 'dbg',
    '--platforms=//bazel/platforms:windows_x86_64',
    '--extra_toolchains=@rust_toolchains//:rw-395021384',
    '--extra_toolchains=@winlibs_mingw64//:mingw_cc_toolchain',
    '--action_env=PATH',
    '--@rules_rust//cargo/settings:debug_std_streams_output_group=true'
)

bazel build @PlatformArgs --jobs=8 --verbose_failures --remote_download_outputs=all //deps/libdd_rc/smoke:windows_msvc_lifecycle
if ($LASTEXITCODE -ne 0) {
    throw "Bazel build failed with exit code $LASTEXITCODE"
}

$lifecycleOutputs = @(bazel cquery @PlatformArgs --output=files //deps/libdd_rc/smoke:windows_msvc_lifecycle)
if ($LASTEXITCODE -ne 0) {
    throw "Lifecycle executable cquery failed with exit code $LASTEXITCODE"
}
Write-Host 'Lifecycle target declared outputs:'
$lifecycleOutputs | ForEach-Object { Write-Host "  $_" }
$lifecycleExecutables = @($lifecycleOutputs | Where-Object { $_ -match '\.exe$' })
if ($lifecycleExecutables.Count -ne 1) {
    throw "Expected one lifecycle executable, found $($lifecycleExecutables.Count): $lifecycleOutputs"
}

$ffiOutputs = @(bazel cquery @PlatformArgs --output=files //deps/libdd_rc/smoke:rc_x509_ffi_windows_dll_msvc)
if ($LASTEXITCODE -ne 0) {
    throw "FFI DLL cquery failed with exit code $LASTEXITCODE"
}
Write-Host 'Transitioned FFI target declared outputs:'
$ffiOutputs | ForEach-Object { Write-Host "  $_" }
$ffiDlls = @($ffiOutputs | Where-Object { $_ -match '\.dll$' })
if ($ffiDlls.Count -eq 0) {
    throw "Could not find the rc-x509 FFI DLL; declared outputs: $ffiOutputs"
}
$ffiDll = @($ffiDlls | Sort-Object -Property FullName)[0]
if ($ffiDll -is [IO.FileInfo]) {
    $ffiDllPath = $ffiDll.FullName
} else {
    $ffiDllPath = $ffiDll
}
Write-Host "Selected rc-x509 FFI DLL: $ffiDllPath"

# aws-lc-fips-sys exposes its CMake install as a tree artifact. The Windows
# runtime DLLs are nested inside that tree, so cquery does not list them as
# separate files. Search that one known output tree rather than recursively
# walking all of bazel-out, which contains tens of thousands of configured
# targets and is prohibitively slow on the Windows runner.
$configurationOutputPaths = @(
    ($lifecycleExecutables[0] -replace '[/\\]bin[/\\].*$', '')
    ($ffiDllPath -replace '[/\\]bin[/\\].*$', '')
) | Sort-Object -Unique
$awsLcOutputPaths = @($configurationOutputPaths | ForEach-Object {
    Join-Path $_ 'bin\external\rules_rs++crate+libdd_rc_crates__aws-lc-fips-sys-0.14.2\aws-lc-fips-sys_build_script.out_dir'
})
Write-Host 'AWS-LC FIPS build-output candidates:'
$awsLcOutputPaths | ForEach-Object { Write-Host "  $_" }
$existingAwsLcOutputPaths = @($awsLcOutputPaths | Where-Object { Test-Path -LiteralPath $_ })
if ($existingAwsLcOutputPaths.Count -eq 0) {
    throw "No AWS-LC FIPS build-output candidate exists: $awsLcOutputPaths"
}
$awsLcDlls = @($existingAwsLcOutputPaths | ForEach-Object {
    Get-ChildItem -LiteralPath $_ -Filter '*.dll' -File -Recurse -ErrorAction Continue
})
if ($awsLcDlls.Count -eq 0) {
    throw "Could not find an AWS-LC FIPS runtime DLL under $existingAwsLcOutputPaths"
}
Write-Host 'AWS-LC FIPS runtime DLLs:'
$awsLcDlls | ForEach-Object { Write-Host "  $($_.FullName)" }

$stageDir = Join-Path $env:TEMP "rc-x509-msvc-lifecycle-$env:CI_JOB_ID"
if (Test-Path -LiteralPath $stageDir) {
    Remove-Item -LiteralPath $stageDir -Recurse -Force
}
New-Item -Path $stageDir -ItemType Directory | Out-Null
Copy-Item -LiteralPath $lifecycleExecutables[0] -Destination (Join-Path $stageDir 'windows_msvc_lifecycle.exe')
Copy-Item -LiteralPath $ffiDllPath -Destination $stageDir
$awsLcDlls | Sort-Object -Property FullName | Group-Object -Property Name | ForEach-Object {
    Copy-Item -LiteralPath $_.Group[0].FullName -Destination $stageDir
}

Write-Host 'Staged Windows lifecycle probe:'
Get-ChildItem -Path $stageDir | Format-Table -AutoSize
& (Join-Path $stageDir 'windows_msvc_lifecycle.exe')
if ($LASTEXITCODE -ne 0) {
    throw "Windows MSVC/MinGW lifecycle probe exited with code $LASTEXITCODE"
}
