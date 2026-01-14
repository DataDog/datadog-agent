<#
.SYNOPSIS

Build the CGO dependencies necessary to build go code or run go linters.

.PARAMETER PythonRuntimes

Python runtime major versions, comma separated.

Default: 3

#>

$ErrorActionPreference = "Stop"

# Run clean to avoid issues with CMakeCache.txt due to moving build roots
& dda inv -- -e rtloader.clean

& dda inv -- -e rtloader.make --install-prefix="$(Get-Location)\dev" --cmake-options='-G \"Unix Makefiles\"'
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader make failed $err"
    [Environment]::Exit($err)
}

& dda inv -- -e rtloader.install
$err = $LASTEXITCODE
Write-Host rtloader install result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader install failed $err"
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