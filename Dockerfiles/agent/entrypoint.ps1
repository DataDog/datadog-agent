$EntrypointPath = "C:\ProgramData\entrypoints"

if (-not (Test-Path env:ENTRYPOINT)) {
    if (Join-Path $EntrypointPath "_default.ps1" | Test-Path) {
        Join-Path $EntrypointPath "_default.ps1" | Invoke-Expression
    } else {
        Write-Host -ForegroundColor Red "No entrypoint is defined."
        exit 1
    }
}

if (Join-Path $EntrypointPath "$env:ENTRYPOINT.ps1" | Test-Path) {
    Join-Path $EntrypointPath "$env:ENTRYPOINT.ps1" | Invoke-Expression
} else {
    Write-Host -ForegroundColor Red "`"$env:ENTRYPOINT`" is not a valid ENTRYPOINT."
    exit 1
}
