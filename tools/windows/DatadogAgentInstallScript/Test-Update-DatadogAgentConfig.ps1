# Test file for Update-DatadogAgentConfig function
# This file contains unit tests for the Update-DatadogAgentConfig function

# Import the main script to get access to the functions
$env:SCRIPT_IMPORT_ONLY = "true"
. (Join-Path $PSScriptRoot "Install-Datadog.ps1")

. (Join-Path $PSScriptRoot "testlib.ps1")

# Define common config templates
$defaultInitialConfig = @(
    "# Test datadog.yaml configuration file",
    "# api_key: placeholder_key",
    "# site: datadoghq.com",
    "# dd_url: https://app.datadoghq.com",
    "# remote_updates: false"
)

# Test Cases

# Test: Update-DatadogAgentConfig with DD_API_KEY set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_API_KEY set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_API_KEY = "test_api_key_123" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "api_key: test_api_key_123",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    ) `
    -AssertMessage "Config should match expected content with API key updated"

# Test: Update-DatadogAgentConfig with DD_SITE set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_SITE set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_SITE = "datadoghq.eu" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "site: datadoghq.eu",
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    ) `
    -AssertMessage "Config should match expected content with site updated"

# Test: Update-DatadogAgentConfig with DD_URL set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_URL set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_URL = "https://custom.datadoghq.com" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "dd_url: https://custom.datadoghq.com",
        "# remote_updates: false"
    ) `
    -AssertMessage "Config should match expected content with URL updated"

# Test: Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to true
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to true" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_REMOTE_UPDATES = "True" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "remote_updates: true"
    ) `
    -AssertMessage "Config should match expected content with remote_updates set to true"

# Test: Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to false
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_REMOTE_UPDATES set to false" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_REMOTE_UPDATES = "False" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "remote_updates: false"
    ) `
    -AssertMessage "Config should match expected content with remote_updates set to false"

# Test: Update-DatadogAgentConfig with all environment variables set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with all environment variables set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{
        DD_API_KEY = "full_test_key"
        DD_SITE = "datadoghq.com"
        DD_URL = "https://app.datadoghq.com"
        DD_REMOTE_UPDATES = "true"
    } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "api_key: full_test_key",
        "site: datadoghq.com",
        "dd_url: https://app.datadoghq.com",
        "remote_updates: true"
    ) `
    -AssertMessage "Config should match expected content with all variables updated"

# Test: Update-DatadogAgentConfig with no environment variables set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with no environment variables set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{} `
    -ExpectedConfig $defaultInitialConfig `
    -AssertMessage "Config should remain unchanged when no env vars are set"

# Test: Update-DatadogAgentConfig with empty string environment variables
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with empty string environment variables" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{
        DD_API_KEY = ""
        DD_SITE = ""
        DD_URL = ""
        DD_REMOTE_UPDATES = ""
    } `
    -ExpectedConfig $defaultInitialConfig `
    -AssertMessage "Config should remain unchanged when env vars are empty strings"

# Test: Update-DatadogAgentConfig with all options set starting from minimal config
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with all options set starting from minimal config" `
    -InitialConfig @(
        "# Minimal datadog.yaml configuration file"
    ) `
    -EnvironmentVariables @{
        DD_API_KEY = "minimal_test_key"
        DD_SITE = "datadoghq.eu"
        DD_URL = "https://custom.datadoghq.eu"
        DD_REMOTE_UPDATES = "true"
    } `
    -ExpectedConfig @(
        "# Minimal datadog.yaml configuration file",
        "api_key: minimal_test_key",
        "site: datadoghq.eu",
        "dd_url: https://custom.datadoghq.eu",
        "remote_updates: true"
    ) `
    -AssertMessage "Config should add all new settings to minimal config file"

# Test: Update-DatadogConfigFile function directly - Add new config line
Test-ConfigUpdate -TestName "Update-DatadogConfigFile function directly - Add new config line" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{} `
    -TestAction { Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: test_value" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false",
        "new_setting: test_value"
    ) `
    -AssertMessage "Config should match expected content with new setting added"

# Test: Update-DatadogConfigFile function directly - Replace existing config line
Test-ConfigUpdate -TestName "Update-DatadogConfigFile function directly - Replace existing config line" `
    -InitialConfig @(
        "# Test config",
        "api_key: old_value",
        "# site: old_site"
    ) `
    -EnvironmentVariables @{} `
    -TestAction { Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: new_value" } `
    -ExpectedConfig @(
        "# Test config",
        "api_key: new_value",
        "# site: old_site"
    ) `
    -AssertMessage "Config should match expected content with api_key replaced"

# Test: Update-DatadogConfigFile function with non-existent file
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

# Test: Update-DatadogConfigFile with file that has EOL at end
Test-ConfigUpdate -TestName "Update-DatadogConfigFile with file that has EOL at end" `
    -TestAction {
        # Set up initial content with EOL
        $initialContent = @(
            "# Test datadog.yaml configuration file",
            "# api_key: placeholder_key",
            "existing_setting: value"
        )
        Set-InitialConfigContent $initialContent

        # Verify file ends with newline by reading as raw bytes
        $rawContent = [System.IO.File]::ReadAllText($global:CurrentTestConfigPath)
        $endsWithNewline = $rawContent[-1] -eq "`n"
        Assert-Equal $true $endsWithNewline "File should end with newline for this test"

        # Perform the actual test operation
        Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: added_value"
    } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "existing_setting: value",
        "new_setting: added_value"
    ) `
    -AssertMessage "Config should match expected content with new setting added to file with EOL"

# Test: Update-DatadogConfigFile with file that does NOT have EOL at end
Test-ConfigUpdate -TestName "Update-DatadogConfigFile with file that does NOT have EOL at end" `
    -TestAction {
        # Create file without trailing newline using helper function
        Set-InitialConfigContentWithoutEOL @(
            "# Test datadog.yaml configuration file",
            "# api_key: placeholder_key",
            "existing_setting: value_no_eol"
        )

        # Verify file doesn't end with newline
        $rawContent = [System.IO.File]::ReadAllText($global:CurrentTestConfigPath)
        $endsWithNewline = $rawContent[-1] -eq "`n"
        Assert-Equal $false $endsWithNewline "File should NOT end with newline for this test"

        # Perform the actual test operation
        Update-DatadogConfigFile "^[ #]*new_setting:.*" "new_setting: added_to_no_eol"
    } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "existing_setting: value_no_eol",
        "new_setting: added_to_no_eol"
    ) `
    -AssertMessage "Config should match expected content with new setting added to file without EOL"

# Test: Update-DatadogConfigFile replacing content in file without EOL
Test-ConfigUpdate -TestName "Update-DatadogConfigFile replacing content in file without EOL" `
    -TestAction {
        # Create file without trailing newline containing content to replace
        Set-InitialConfigContentWithoutEOL @(
            "# Test config",
            "api_key: old_key_no_eol",
            "site: existing_site"
        )

        # Perform the actual test operation
        Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: replaced_key"
    } `
    -ExpectedConfig @(
        "# Test config",
        "api_key: replaced_key",
        "site: existing_site"
    ) `
    -AssertMessage "Config should match expected content with api_key replaced in file without EOL"

# Test: Adds tags block to fresh config
Test-ConfigUpdate -TestName "Adds tags block to fresh config" `
    -InitialConfig @(
        "#"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:prod,team:sre" } `
    -ExpectedConfig @(
        "#",
        "tags:",
        "  - env:prod",
        "  - team:sre"
    ) `
    -AssertMessage "Should add new tags block when none exists"

# Test: Replaces existing tags block
Test-ConfigUpdate -TestName "Replaces existing tags block" `
    -InitialConfig @(
        "# Existing install",
        "tags:",
        "  - oldtag:legacy",
        "  - team:old"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:qa,team:platform" } `
    -ExpectedConfig @(
        "# Existing install",
        "tags:",
        "  - env:qa",
        "  - team:platform"
    ) `
    -AssertMessage "Should replace existing tags block with new values"

# Test: Rerun updates tags block
Test-ConfigUpdate -TestName "Rerun updates tags block" `
    -InitialConfig @(
        "# Config from earlier run",
        "tags:",
        "  - env:staging",
        "  - team:infra"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:prod,team:core" } `
    -ExpectedConfig @(
        "# Config from earlier run",
        "tags:",
        "  - env:prod",
        "  - team:core"
    ) `
    -AssertMessage "Should overwrite tags block on rerun"

# Test: Update-DatadogAgentConfig with DD_LOGS_ENABLED set
Test-ConfigUpdate -TestName "Update-DatadogAgentConfig with DD_LOGS_ENABLED set" `
    -InitialConfig $defaultInitialConfig `
    -EnvironmentVariables @{ DD_LOGS_ENABLED = "true" } `
    -ExpectedConfig @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false",
        "logs_enabled: true"
    ) `
    -AssertMessage "Config should match expected content with logs_enabled set to true"


# Test: does not modify indented tags block
Test-ConfigUpdate -TestName "does not modify indented tags blocks" `
    -InitialConfig @(
        "# Config from earlier run",
        "other_data:",
        "  tags:",
        "    - other tags",
        "tags:",
        "  - env:staging",
        "  - team:infra"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:prod,team:core" } `
    -ExpectedConfig @(
        "# Config from earlier run",
        "other_data:",
        "  tags:",
        "    - other tags",
        "tags:",
        "  - env:prod",
        "  - team:core"
    ) `
    -AssertMessage "Should not modify indented tags blocks"


# Test: replaces inline array form of tags
Test-ConfigUpdate -TestName "replaces inline array form of tags" `
    -InitialConfig @(
        "# YAML with inline tags array",
        "tags: ['env:staging', 'team:infra']"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:prod,team:core" } `
    -ExpectedConfig @(
        "# YAML with inline tags array",
        "tags:",
        "  - env:prod",
        "  - team:core"
    ) `
    -AssertMessage "Should replace inline tags array with YAML block format"


#Test: tags after comment block
Test-ConfigUpdate -TestName "tags after comment block" `
    -InitialConfig @(
        "# Config from earlier run",
        "# tags:",
        "#   - team:infra",
        "#   - <TAG_KEY>:<TAG_VALUE>",
        "tags:",
        "  - env:staging",
        "  - team:infra"
    ) `
    -EnvironmentVariables @{ DD_TAGS = "env:prod,team:core" } `
    -ExpectedConfig @(
        "# Config from earlier run",
        "# tags:",
        "#   - team:infra",
        "#   - <TAG_KEY>:<TAG_VALUE>",
        "tags:",
        "  - env:prod",
        "  - team:core"
    ) `
    -AssertMessage "Should not modify tags in comment blocks"



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
