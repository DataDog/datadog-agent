# Test Runner for Update-DatadogAgentConfig Unit Tests
# This script runs the unit tests and provides formatted output

Write-Host "==========================================="
Write-Host "Datadog Agent Config Update Function Tests"
Write-Host "==========================================="
Write-Host ""

# Check if the test file exists
$testFile = Join-Path $PSScriptRoot "Test-Update-DatadogAgentConfig.ps1"
$mainScript = Join-Path $PSScriptRoot "Install-Datadog.ps1"

if (-not (Test-Path $testFile)) {
    Write-Host "Test file not found: $testFile" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path $mainScript)) {
    Write-Host "Main script not found: $mainScript" -ForegroundColor Red
    exit 1
}

Write-Host "Running tests from: $testFile" -ForegroundColor Cyan
Write-Host "Testing functions from: $mainScript" -ForegroundColor Cyan
Write-Host ""

try {
    # Execute the test file
    & $testFile
    $exitCode = $LASTEXITCODE

    Write-Host ""
    if ($exitCode -eq 0) {
        Write-Host "All tests completed successfully!" -ForegroundColor Green
    } else {
        Write-Host "Tests completed with failures. Exit code: $exitCode" -ForegroundColor Red
    }

    exit $exitCode
} catch {
    Write-Host "Error running tests: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
