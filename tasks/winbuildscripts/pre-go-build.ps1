<#
.SYNOPSIS

Build the CGO dependencies necessary to build go code or run go linters.

.PARAMETER Architecture

Build architecture, x86 or x64.

Default: x64

.PARAMETER PythonRuntimes

Python runtime major versions, comma separated.

Default: 3

#>

param(
    [Parameter(Mandatory=$false)][string]$Architecture = 'x64',
    [Parameter(Mandatory=$false)][string]$PythonRuntimes = '3'
)

$ErrorActionPreference = "Stop"

& inv -e rtloader.make --python-runtimes="$PythonRuntimes" --install-prefix="$(Get-Location)\dev" --cmake-options='-G \"Unix Makefiles\"' --arch $Architecture
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader make failed $err"
    [Environment]::Exit($err)
}

& inv -e rtloader.install
$err = $LASTEXITCODE
Write-Host rtloader install result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader install failed $err"
    [Environment]::Exit($err)
}

& inv -e build-messagetable --arch="$Architecture"
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "build-messagetable failed $err"
    [Environment]::Exit($err)
}
