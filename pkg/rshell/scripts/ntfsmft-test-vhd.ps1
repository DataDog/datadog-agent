<#
Creates or destroys a small, isolated NTFS VHDX for ntfsmft integration tests.

Example:
  $fixture = .\scripts\ntfsmft-test-vhd.ps1 -Action Create
  $env:RSHELL_NTFSDU_TEST_ROOT = $fixture.TestRoot
  go test -race ./builtins/internal/ntfsmft
  .\scripts\ntfsmft-test-vhd.ps1 -Action Destroy -VhdPath $fixture.VhdPath
#>

[CmdletBinding()]
param(
    [ValidateSet('Create', 'Destroy')]
    [string]$Action = 'Create',

    [ValidatePattern('^[A-Za-z]$')]
    [string]$DriveLetter = 'R',

    [string]$VhdDirectory = [IO.Path]::GetTempPath(),

    [string]$VhdPath,

    [switch]$AsJson
)

$ErrorActionPreference = 'Stop'
$DriveLetter = $DriveLetter.ToUpperInvariant()

function Invoke-DiskPartScript([string]$Content, [string]$Directory, [bool]$Quiet) {
    $scriptPath = Join-Path $Directory "diskpart-$([guid]::NewGuid().ToString('N')).txt"
    try {
        $Content | Set-Content -LiteralPath $scriptPath -Encoding ascii
        if ($Quiet) {
            & diskpart.exe /s $scriptPath | Out-Null
        } else {
            & diskpart.exe /s $scriptPath | ForEach-Object { Write-Host $_ }
        }
        if ($LASTEXITCODE -ne 0) {
            throw 'DiskPart failed'
        }
    } finally {
        Remove-Item -LiteralPath $scriptPath -Force -ErrorAction SilentlyContinue
    }
}

if ($Action -eq 'Destroy') {
    if ([string]::IsNullOrWhiteSpace($VhdPath)) {
        throw 'Destroy requires -VhdPath'
    }
    if (-not (Test-Path -LiteralPath $VhdPath)) {
        return
    }
    $fullPath = [IO.Path]::GetFullPath($VhdPath)
    $fileName = [IO.Path]::GetFileName($fullPath)
    if ($fileName -notmatch '^rshell-ntfsmft-[0-9a-f]{32}\.vhdx$') {
        throw "Refusing to delete non-test VHD: $fullPath"
    }
    Invoke-DiskPartScript @"
select vdisk file="$fullPath"
detach vdisk noerr
exit
"@ ([IO.Path]::GetTempPath()) $AsJson
    Remove-Item -LiteralPath $fullPath -Force
    return
}

if (-not [string]::IsNullOrWhiteSpace($VhdPath)) {
    throw 'Create generates its VHD path; do not pass -VhdPath'
}

$baseDirectory = [IO.Path]::GetFullPath($VhdDirectory)
if (-not (Test-Path -LiteralPath $baseDirectory -PathType Container)) {
    throw "VHD directory does not exist: $baseDirectory"
}
if (Get-Volume -DriveLetter $DriveLetter -ErrorAction SilentlyContinue) {
    throw "Refusing to use $DriveLetter`: because it is already assigned"
}

$id = [guid]::NewGuid().ToString('N')
$fullPath = [IO.Path]::GetFullPath((Join-Path $baseDirectory "rshell-ntfsmft-$id.vhdx"))
if (-not $fullPath.StartsWith($baseDirectory + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing VHD path outside requested directory: $fullPath"
}
if (Test-Path -LiteralPath $fullPath) {
    throw "Refusing to overwrite existing VHD: $fullPath"
}

$label = "rshell-$($id.Substring(0, 16))"
try {
    Invoke-DiskPartScript @"
create vdisk file="$fullPath" maximum=2048 type=expandable
select vdisk file="$fullPath"
attach vdisk
create partition primary
format quick fs=ntfs label="$label"
assign letter=$DriveLetter
detail vdisk
exit
"@ $baseDirectory $AsJson

    $volume = Get-Volume -DriveLetter $DriveLetter -ErrorAction Stop
    if ($volume.FileSystem -ne 'NTFS' -or $volume.FileSystemLabel -ne $label) {
        throw "Mounted test drive identity check failed: filesystem=$($volume.FileSystem) label=$($volume.FileSystemLabel)"
    }
} catch {
    if (Test-Path -LiteralPath $fullPath) {
        try {
            Invoke-DiskPartScript @"
select vdisk file="$fullPath"
detach vdisk noerr
exit
"@ $baseDirectory $AsJson
            Remove-Item -LiteralPath $fullPath -Force
        } catch {
            Write-Warning "Could not detach failed test VHD ${fullPath}: $_"
        }
    }
    throw
}

$fixture = [pscustomobject]@{
    VhdPath  = $fullPath
    TestRoot = "$DriveLetter`:\"
    Label    = $label
}
if ($AsJson) {
    $fixture | ConvertTo-Json -Compress
} else {
    $fixture
}
