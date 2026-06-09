<#
.SYNOPSIS

Build the CGO dependencies necessary to build go code or run go linters.

.PARAMETER PythonRuntimes

Python runtime major versions, comma separated.

Default: 3

#>

$ErrorActionPreference = "Stop"

# Build and install rtloader using Bazel
Write-Host "Building rtloader with Bazel..."
& bazel run //rtloader:install -- --destdir="$(Get-Location)"
$err = $LASTEXITCODE
Write-Host "Bazel rtloader install result is $err"
if($err -ne 0){
    Write-Host -ForegroundColor Red "Bazel rtloader install failed $err"
    [Environment]::Exit($err)
}

& dda inv -- -e build-messagetable
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "build-messagetable failed $err"
    [Environment]::Exit($err)
}

# Build libdatadog-interop.dll for software inventory tests
& dda inv -- -e msi.build-datadog-interop
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "msi.build-datadog-interop failed $err"
    [Environment]::Exit($err)
}