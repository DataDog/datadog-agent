param(
    [Parameter(Mandatory=$true)]
    [string]$destdir,

    [Parameter(Mandatory=$false)]
    [string]$embedded_ssl_dir
)

$ErrorActionPreference = "Stop"

# If embedded_ssl_dir is not provided, derive it from destdir
if (-not $embedded_ssl_dir) {
    $embedded_ssl_dir = Join-Path $destdir "embedded\ssl"
}

$opensslCnf = Join-Path $destdir "embedded\ssl\openssl.cnf"

# Replace {{embedded_ssl_dir}} with the actual path in openssl.cnf
if (Test-Path $opensslCnf) {
    $content = Get-Content $opensslCnf -Raw
    $content = $content -replace [regex]::Escape("{{embedded_ssl_dir}}"), $embedded_ssl_dir
    Set-Content -Path $opensslCnf -Value $content -NoNewline
    Write-Host "Updated: $opensslCnf"
} else {
    Write-Warning "$opensslCnf not found"
}

Write-Host "Configuration complete."

