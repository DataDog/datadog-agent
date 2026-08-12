param(
    [Parameter(Mandatory = $true)]
    [string]$Source,

    [Parameter(Mandatory = $true)]
    [string]$InstallRoot,

    [switch]$Force
)

$ErrorActionPreference = "Stop"

$bash = Join-Path $InstallRoot "usr\bin\bash.exe"
if (-not $Force -and (Test-Path -LiteralPath $bash)) {
    exit 0
}

if ($Force -and (Test-Path -LiteralPath $InstallRoot)) {
    Remove-Item -LiteralPath $InstallRoot -Recurse -Force
}

New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null

# robocopy: exit codes 0-7 mean success.
& robocopy $Source $InstallRoot /MIR /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
$code = $LASTEXITCODE
if ($code -ge 8) {
    throw "robocopy failed with exit code $code"
}

if (-not (Test-Path -LiteralPath $bash)) {
    throw "MSYS2 install finished but bash.exe is missing at $bash"
}

exit 0
