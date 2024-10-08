#!/usr/bin/env pwsh

if ($args.Length -lt 1) {
    Write-Host "Usage: $($MyInvocation.InvocationName) <command> [arguments]"
    Write-Host "The script will execute the provided commands and retry in case of failures"
    Exit 1
}

$NB_RETRIES = 5

$commandArgs = $args[1..($args.Length - 1)]
for ($i = 1; $i -le $NB_RETRIES; $i++) {
    & $args[0] @commandArgs
    $errorCode = $LASTEXITCODE
    if ($errorCode -eq 0) {
        Exit 0
    } 
    Write-Host "Attempt #$i failed with error code $errorCode"
    if ($i -lt $NB_RETRIES) {
        Start-Sleep -Seconds ($i * $i)
    }
}

Exit $errorCode
