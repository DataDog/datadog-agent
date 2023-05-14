
# Don't allow starting without an apikey set
if (-not (Test-Path env:DD_API_KEY)) { 
    Write-Output ""
    Write-Output "=================================================================================="
    Write-Output "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    Write-Output "=================================================================================="
    Write-Output ""
    exit 1
}
