
# Don't allow starting without an apikey set
if ((-not (Test-Path env:DD_API_KEY)) -and (-not (Test-Path env:DD_API_KEY_FILE))) {
    Write-Output ""
    Write-Output "========================================================================================================="
    Write-Output "You must set either DD_API_KEY or DD_API_KEY_FILE environment variable to run the Datadog Agent container"
    Write-Output "========================================================================================================="
    Write-Output ""
    exit 1
}
