<#
.SYNOPSIS

Build the CGO dependencies necessary to build go code or run go linters.

.PARAMETER PythonRuntimes

Python runtime major versions, comma separated.

Default: 3

#>

$ErrorActionPreference = "Stop"

& inv -e rtloader.make --install-prefix="$(Get-Location)\dev" --cmake-options='-G \"Unix Makefiles\"'
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

& inv -e build-messagetable
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "build-messagetable failed $err"
    [Environment]::Exit($err)
}
