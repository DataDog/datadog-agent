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
    # rules_rust may expose only the import library through DefaultInfo even
    # though the same rustc action produced the runtime DLL. Keep this
    # diagnostic moving while printing enough evidence to model it explicitly
    # in the subsequent packaging change.
    $ffiDlls = @(
        Get-ChildItem -Path bazel-out -Filter 'rc_x509_ffi*.dll' -File -Recurse -ErrorAction Continue |
            Where-Object { $_.FullName -match 'x64_windows-dbg' }
    )
}
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
# separate files. The build's remote_download_outputs=all makes them available
# for this explicit staging probe.
$awsLcDlls = @(
    Get-ChildItem -Path bazel-out -Filter 'aws_lc_fips_0_14_2_*.dll' -File -Recurse -ErrorAction Continue |
        Where-Object { $_.FullName -match 'aws-lc-fips-sys_build_script\.out_dir' }
)
if ($awsLcDlls.Count -eq 0) {
    Write-Host 'DLLs found beneath AWS-LC FIPS build outputs:'
    Get-ChildItem -Path bazel-out -Filter '*.dll' -File -Recurse -ErrorAction Continue |
        Where-Object { $_.FullName -match 'aws-lc-fips' } |
        Select-Object -ExpandProperty FullName
    throw 'Could not find an AWS-LC FIPS runtime DLL under bazel-out'
}

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
