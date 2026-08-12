param(
    [Parameter(Mandatory = $true)]
    [string]$Source,

    [Parameter(Mandatory = $true)]
    [string]$InstallRoot,

    [switch]$Force
)

$ErrorActionPreference = "Stop"

$Source = (Resolve-Path -LiteralPath $Source).Path
$InstallRoot = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallRoot)
$bash = Join-Path $InstallRoot "usr\bin\bash.exe"

if (-not $Force -and (Test-Path -LiteralPath $bash)) {
    exit 0
}

# download_and_extract uses strip_prefix=msys64; tolerate a nested layout if not stripped.
$sourceRoot = $Source
$nested = Join-Path $Source "msys64"
if (-not (Test-Path -LiteralPath (Join-Path $Source "usr\bin\bash.exe")) -and (Test-Path -LiteralPath (Join-Path $nested "usr\bin\bash.exe"))) {
    $sourceRoot = $nested
}

if (-not (Test-Path -LiteralPath (Join-Path $sourceRoot "usr\bin\bash.exe"))) {
    throw "MSYS2 source tree has no usr\bin\bash.exe under $sourceRoot"
}

if ($Force -and (Test-Path -LiteralPath $InstallRoot)) {
    Remove-Item -LiteralPath $InstallRoot -Recurse -Force
}

New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null

# robocopy: exit codes 0-7 mean success.
& robocopy $sourceRoot $InstallRoot /MIR /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
$code = $LASTEXITCODE
if ($code -ge 8) {
    throw "robocopy from $sourceRoot to $InstallRoot failed with exit code $code"
}

if (-not (Test-Path -LiteralPath $bash)) {
    throw "MSYS2 install finished but bash.exe is missing at $bash"
}

exit 0
