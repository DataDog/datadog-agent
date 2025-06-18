# Test file for Update-DatadogAgentConfig function
# This file contains unit tests for the Update-DatadogAgentConfig function

# Import the main script to get access to the functions
$env:SCRIPT_IMPORT_ONLY = "true"
. (Join-Path $PSScriptRoot "Install-Datadog.ps1")

. (Join-Path $PSScriptRoot "testlib.ps1")

# Test Cases

# Test 1: Update-DatadogAgentConfig with DD_API_KEY set
Start-Test "Update-DatadogAgentConfig with DD_API_KEY set"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_API_KEY = "test_api_key_123"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "api_key: test_api_key_123",
        "# site: datadoghq.com", 
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with API key updated"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with DD_API_KEY set"

# Test 2: Update-DatadogAgentConfig with DD_SITE set
Start-Test "Update-DatadogAgentConfig with DD_SITE set"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_SITE = "datadoghq.eu"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "site: datadoghq.eu", 
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with site updated"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with DD_SITE set"

# Test 3: Update-DatadogAgentConfig with DD_URL set
Start-Test "Update-DatadogAgentConfig with DD_URL set"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_URL = "https://custom.datadoghq.com"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com", 
        "dd_url: https://custom.datadoghq.com",
        "# remote_updates: false"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with URL updated"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with DD_URL set"

# Test 4: Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to true
Start-Test "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to true"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_REMOTE_UPDATES = "True"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com", 
        "# dd_url: https://app.datadoghq.com",
        "remote_updates: true"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with remote_updates set to true"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to true"

# Test 5: Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to false
Start-Test "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to false"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_REMOTE_UPDATES = "False"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com", 
        "# dd_url: https://app.datadoghq.com",
        "remote_updates: false"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with remote_updates set to false"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to false"

# Test 6: Update-DatadogAgentConfig with all environment variables set
Start-Test "Update-DatadogAgentConfig with all environment variables set"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_API_KEY = "full_test_key"
$env:DD_SITE = "datadoghq.com"
$env:DD_URL = "https://app.datadoghq.com"
$env:DD_REMOTE_UPDATES = "true"
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "api_key: full_test_key",
        "site: datadoghq.com", 
        "dd_url: https://app.datadoghq.com",
        "remote_updates: true"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with all variables updated"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with all environment variables set"

# Test 7: Update-DatadogAgentConfig with no environment variables set
Start-Test "Update-DatadogAgentConfig with no environment variables set"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

try {
    Update-DatadogAgentConfig
    $expectedContent = $initialContent
    Assert-ConfigEquals $expectedContent "Config should remain unchanged when no env vars are set"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with no environment variables set"

# Test 8: Update-DatadogAgentConfig with empty string environment variables
Start-Test "Update-DatadogAgentConfig with empty string environment variables"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

$env:DD_API_KEY = ""
$env:DD_SITE = ""
$env:DD_URL = ""
$env:DD_REMOTE_UPDATES = ""
try {
    Update-DatadogAgentConfig
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com", 
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    )
    Assert-ConfigEquals $expectedContent "Config should remain unchanged when env vars are empty strings"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogAgentConfig with empty string environment variables"

# Test 9: Update-DatadogConfigFile function directly - Add new config line
Start-Test "Update-DatadogConfigFile function directly - Add new config line"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com", 
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)
Set-InitialConfigContent $initialContent

try {
    Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: test_value"
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com", 
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false",
        "new_setting: test_value"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with new setting added"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogConfigFile function directly - Add new config line"

# Test 10: Update-DatadogConfigFile function directly - Replace existing config line
Start-Test "Update-DatadogConfigFile function directly - Replace existing config line"
$initialContent = @(
    "# Test config",
    "api_key: old_value",
    "# site: old_site"
)
Set-InitialConfigContent $initialContent

try {
    Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: new_value"
    $expectedContent = @(
        "# Test config",
        "api_key: new_value",
        "# site: old_site"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with api_key replaced"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogConfigFile function directly - Replace existing config line"

# Test 11: Update-DatadogConfigFile function with non-existent file
Start-Test "Update-DatadogConfigFile function with non-existent file"
$global:CurrentTestConfigPath = "C:\NonExistent\datadog.yaml"
try {
    Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: test"
    Write-Host "  ✗ Should have thrown exception for non-existent file" -ForegroundColor Red
    $global:CurrentTestPassed = $false
} catch {
    if ($_.Exception.Message -eq "datadog.yaml doesn't exist") {
        Write-Host "  ✓ Correctly threw exception for non-existent file" -ForegroundColor Green
    } else {
        Write-Host "  ✗ Threw wrong exception: $($_.Exception.Message)" -ForegroundColor Red
        $global:CurrentTestPassed = $false
    }
}
End-Test "Update-DatadogConfigFile function with non-existent file"

# Test 12: Update-DatadogConfigFile with file that has EOL at end
Start-Test "Update-DatadogConfigFile with file that has EOL at end"
$initialContent = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "existing_setting: value"
)
Set-InitialConfigContent $initialContent

try {
    # Verify file ends with newline by reading as raw bytes
    $rawContent = [System.IO.File]::ReadAllText($global:CurrentTestConfigPath)
    $endsWithNewline = $rawContent[-1] -eq "`n"
    Assert-Equal $true $endsWithNewline "File should end with newline for this test"
    
    Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: added_value"
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "existing_setting: value",
        "new_setting: added_value"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with new setting added to file with EOL"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogConfigFile with file that has EOL at end"

# Test 13: Update-DatadogConfigFile with file that does NOT have EOL at end
Start-Test "Update-DatadogConfigFile with file that does NOT have EOL at end"
# Create file without trailing newline using raw file operations
$contentWithoutEOL = "# Test datadog.yaml configuration file`r`n# api_key: placeholder_key`r`nexisting_setting: value_no_eol"
[System.IO.File]::WriteAllText($global:CurrentTestConfigPath, $contentWithoutEOL)

try {
    # Verify file doesn't end with newline
    $rawContent = [System.IO.File]::ReadAllText($global:CurrentTestConfigPath)
    $endsWithNewline = $rawContent[-1] -eq "`n"
    Assert-Equal $false $endsWithNewline "File should NOT end with newline for this test"
    
    Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: added_to_no_eol"
    $expectedContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "existing_setting: value_no_eol",
        "new_setting: added_to_no_eol"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with new setting added to file without EOL"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogConfigFile with file that does NOT have EOL at end"

# Test 14: Update-DatadogConfigFile replacing content in file without EOL
Start-Test "Update-DatadogConfigFile replacing content in file without EOL"
# Create file without trailing newline containing content to replace
$contentToReplace = "# Test config`r`napi_key: old_key_no_eol`r`nsite: existing_site"
[System.IO.File]::WriteAllText($global:CurrentTestConfigPath, $contentToReplace)

try {
    Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: replaced_key"
    $expectedContent = @(
        "# Test config",
        "api_key: replaced_key",
        "site: existing_site"
    )
    Assert-ConfigEquals $expectedContent "Config should match expected content with api_key replaced in file without EOL"
} catch {
    Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
    $global:CurrentTestPassed = $false
}
End-Test "Update-DatadogConfigFile replacing content in file without EOL"

# Cleanup
Cleanup-Tests

# Print test results summary
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "Test Results Summary" -ForegroundColor Cyan
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "Total Tests: $global:TestCount" -ForegroundColor White
Write-Host "Passed: $global:PassedTests" -ForegroundColor Green
Write-Host "Failed: $global:FailedTests" -ForegroundColor Red

if ($global:FailedTests -eq 0) {
    Write-Host "All tests passed! ✓" -ForegroundColor Green
    exit 0
} else {
    Write-Host "Some tests failed. ✗" -ForegroundColor Red
    exit 1
}
