# Test framework variables
$global:TestResults = @()
$global:TestCount = 0
$global:PassedTests = 0
$global:FailedTests = 0

# Test state variables
$global:TempConfigFiles = @()
$global:CurrentTestConfigPath = ""
$global:CurrentTestPassed = $true

# Override Get-DatadogConfigPath to return our test-specific config file
function Get-DatadogConfigPath() {
    return $global:CurrentTestConfigPath
}

# Test helper functions
function Start-Test($TestName) {
    $global:TestCount++
    Write-Host "Running Test $global:TestCount`: $TestName" -ForegroundColor Cyan

    # Reset test status
    $global:CurrentTestPassed = $true

    # Create a unique temporary config file for this test
    $tempFile = [System.IO.Path]::GetTempFileName()
    $global:CurrentTestConfigPath = $tempFile
    $global:TempConfigFiles += $tempFile

    # Create initial config file content
    $initialContent = @(
        "# Test datadog.yaml configuration file",
        "# api_key: placeholder_key",
        "# site: datadoghq.com",
        "# dd_url: https://app.datadoghq.com",
        "# remote_updates: false"
    )
    Set-Content -Path $tempFile -Value $initialContent

    # Clear environment variables
    $env:DD_API_KEY = $null
    $env:DD_SITE = $null
    $env:DD_URL = $null
    $env:DD_REMOTE_UPDATES = $null
    $env:DD_TAGS = $null
    $env:DD_LOGS_ENABLED = $null
}

function Get-TestConfigContent() {
    if (Test-Path $global:CurrentTestConfigPath) {
        return Get-Content -Path $global:CurrentTestConfigPath
    }
    return @()
}

function Set-InitialConfigContent($Content) {
    Set-Content -Path $global:CurrentTestConfigPath -Value $Content
}

# Helper function to create config file without trailing EOL
function Set-InitialConfigContentWithoutEOL($Content) {
    # Join content with CRLF and write without trailing newline
    $contentString = $Content -join "`r`n"
    [System.IO.File]::WriteAllText($global:CurrentTestConfigPath, $contentString)

    $rawContent = [System.IO.File]::ReadAllText($global:CurrentTestConfigPath)
    $endsWithNewline = $rawContent[-1] -eq "`n"
    if ($endsWithNewline) {
        throw "File should NOT end with newline for this test"
    }
}

# Helper function to streamline common test pattern
function Test-ConfigUpdate {
    param(
        [Parameter(Mandatory=$true)]
        [string]$TestName,

        [Parameter(Mandatory=$false)]
        [string[]]$InitialConfig,

        [Parameter(Mandatory=$false)]
        [hashtable]$EnvironmentVariables = @{},

        [Parameter(Mandatory=$true)]
        [string[]]$ExpectedConfig,

        [Parameter(Mandatory=$false)]
        [scriptblock]$TestAction = { Update-DatadogAgentConfig },

        [Parameter(Mandatory=$false)]
        [string]$AssertMessage = "Config should match expected content"
    )

    Start-Test $TestName

    # Set initial config
    Set-InitialConfigContent $InitialConfig

    # Set environment variables
    foreach ($key in $EnvironmentVariables.Keys) {
        Set-Item -Path "env:$key" -Value $EnvironmentVariables[$key]
    }

    try {
        # Execute the test action
        & $TestAction

        # Assert the result
        Assert-ConfigEquals $ExpectedConfig $AssertMessage
    } catch {
        Write-Host "  ✗ Test threw exception: $($_.Exception.Message)" -ForegroundColor Red
        $global:CurrentTestPassed = $false
    }

    End-Test $TestName
}

function Assert-ConfigContains($ExpectedLine, $Message) {
    $content = Get-TestConfigContent
    if ($content -contains $ExpectedLine) {
        Write-Host "  ✓ $Message" -ForegroundColor Green
        return $true
    } else {
        Write-Host "  ✗ $Message" -ForegroundColor Red
        Write-Host "    Expected line: $ExpectedLine" -ForegroundColor Red
        Write-Host "    Actual content:" -ForegroundColor Red
        $content | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
        $global:CurrentTestPassed = $false
        return $false
    }
}

function Assert-ConfigMatches($Pattern, $Message) {
    $content = Get-TestConfigContent
    $matches = $content | Select-String $Pattern
    if ($matches.Count -gt 0) {
        Write-Host "  ✓ $Message" -ForegroundColor Green
        return $true
    } else {
        Write-Host "  ✗ $Message" -ForegroundColor Red
        Write-Host "    Expected pattern: $Pattern" -ForegroundColor Red
        Write-Host "    Actual content:" -ForegroundColor Red
        $content | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
        $global:CurrentTestPassed = $false
        return $false
    }
}

function Assert-ConfigEquals($ExpectedContent, $Message) {
    $actualContent = Get-TestConfigContent

    # Compare lengths first
    if ($ExpectedContent.Count -ne $actualContent.Count) {
        Write-Host "  ✗ $Message" -ForegroundColor Red
        Write-Host "    Expected $($ExpectedContent.Count) lines, got $($actualContent.Count) lines" -ForegroundColor Red
        Write-Host "    Expected content:" -ForegroundColor Red
        $ExpectedContent | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
        Write-Host "    Actual content:" -ForegroundColor Red
        $actualContent | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
        $global:CurrentTestPassed = $false
        return $false
    }

    # Compare line by line
    for ($i = 0; $i -lt $ExpectedContent.Count; $i++) {
        if ($ExpectedContent[$i] -ne $actualContent[$i]) {
            Write-Host "  ✗ $Message" -ForegroundColor Red
            Write-Host "    Line $($i + 1) differs:" -ForegroundColor Red
            Write-Host "      Expected: '$($ExpectedContent[$i])'" -ForegroundColor Red
            Write-Host "      Actual:   '$($actualContent[$i])'" -ForegroundColor Red
            Write-Host "    Full expected content:" -ForegroundColor Red
            $ExpectedContent | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
            Write-Host "    Full actual content:" -ForegroundColor Red
            $actualContent | ForEach-Object { Write-Host "      $_" -ForegroundColor Red }
            $global:CurrentTestPassed = $false
            return $false
        }
    }

    Write-Host "  ✓ $Message" -ForegroundColor Green
    return $true
}

function Assert-Equal($Expected, $Actual, $Message) {
    if ($Expected -eq $Actual) {
        Write-Host "  ✓ $Message" -ForegroundColor Green
        return $true
    } else {
        Write-Host "  ✗ $Message" -ForegroundColor Red
        Write-Host "    Expected: $Expected" -ForegroundColor Red
        Write-Host "    Actual: $Actual" -ForegroundColor Red
        $global:CurrentTestPassed = $false
        return $false
    }
}

function Assert-Contains($Collection, $Item, $Message) {
    if ($Collection -contains $Item) {
        Write-Host "  ✓ $Message" -ForegroundColor Green
        return $true
    } else {
        Write-Host "  ✗ $Message" -ForegroundColor Red
        Write-Host "    Collection: $($Collection -join ', ')" -ForegroundColor Red
        Write-Host "    Looking for: $Item" -ForegroundColor Red
        $global:CurrentTestPassed = $false
        return $false
    }
}

function End-Test($TestName) {
    $testPassed = $global:CurrentTestPassed
    $global:TestResults += @{
        Name = $TestName
        Passed = $testPassed
    }

    if ($testPassed) {
        $global:PassedTests++
        Write-Host "Test Passed: $TestName" -ForegroundColor Green
    } else {
        $global:FailedTests++
        Write-Host "Test Failed: $TestName" -ForegroundColor Red
    }
    Write-Host ""
}

function Cleanup-Tests() {
    Write-Host "Cleaning up temporary files..." -ForegroundColor Yellow
    foreach ($tempFile in $global:TempConfigFiles) {
        if (Test-Path $tempFile) {
            Remove-Item -Path $tempFile -Force -ErrorAction SilentlyContinue
        }
    }
    $global:TempConfigFiles = @()
}
