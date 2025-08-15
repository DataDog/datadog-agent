<#
.SYNOPSIS
Invoke the unit tests.

.DESCRIPTION
Invoke the unit tests, with options to configure the build environment.

Runs unit tests for rtloader, Go, and MSI .NET.

Can upload coverage reports to Codecov and test results to Datadog CI.

.PARAMETER BuildOutOfSource
Specifies whether to build out of source. Default is $false.

Use this option in the CI to keep the job directory clean and avoid conflicts/stale data.
Use this option in Hyper-V based containers to improve build performance.

.PARAMETER InstallDeps
Specifies whether to install dependencies (python requirements, go deps, etc.). Default is $true.

.PARAMETER CheckGoVersion
Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable GO_VERSION_CHECK or $true if the environment variable is not set.

.PARAMETER UploadCoverage
Specifies whether to upload coverage reports to Codecov. Default is $false.

Requires the CODECOV_TOKEN environment variable to be set.

.PARAMETER UploadTestResults
Specifies whether to upload test results to Datadog CI. Default is $false.

Requires the API_KEY_ORG2 environment variable to be set.

Requires JUNIT_TAR environment variable to be set.

#>
param(
    [bool] $BuildOutOfSource = $false,
    [nullable[bool]] $CheckGoVersion,
    [bool] $InstallDeps = $true,
    [bool] $UploadCoverage = $false,
    [bool] $UploadTestResults = $false
)

. "$PSScriptRoot\common.ps1"

Invoke-BuildScript `
    -BuildOutOfSource $BuildOutOfSource `
    -InstallDeps $InstallDeps `
    -InstallTestingDeps $InstallDeps `
    -CheckGoVersion $CheckGoVersion `
    -Command {

    # pre-reqs
    & {
        # Check required environment variables
        if ([string]::IsNullOrEmpty($Env:TEST_EMBEDDED_PY3)) {
            Write-Host -ForegroundColor Red "TEST_EMBEDDED_PY3 environment variable is required for running embedded Python 3 tests"
            exit 1
        }
        if ($UploadCoverage) {
            if ([string]::IsNullOrEmpty($Env:CODECOV_TOKEN)) {
                Write-Host -ForegroundColor Red "CODECOV_TOKEN environment variable is required for uploading coverage reports to Codecov"
                exit 1
            }
        }
        if ($UploadTestResults) {
            if ([string]::IsNullOrEmpty($Env:API_KEY_ORG2)) {
                Write-Host -ForegroundColor Red "API_KEY_ORG2 environment variable is required for junit upload to Datadog CI"
                exit 1
            }
            if ([string]::IsNullOrEmpty($Env:JUNIT_TAR)) {
                Write-Host -ForegroundColor Red "JUNIT_TAR environment variable is required for junit upload to Datadog CI"
                exit 1
            }
        }
        # Add the dev\lib directory to the PATH so that the go tests can find the rtloader DLL
        # TODO: This is a weird way to load the rtloader DLLs
        $env:PATH="$(Get-Location)\dev\lib;$env:PATH"
        # Create ddagentuser for secrets tests if it doesn't already exist
        if (-not (Get-LocalUser -Name "ddagentuser" -ErrorAction SilentlyContinue)) {
            $Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
            New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password
        }
        # Generate the datadog.yaml config file to be used in integration tests
        & dda inv -- -e agent.generate-config --build-type="agent-py2py3" --output-file="./datadog.yaml"
        # Build inputs needed for go builds
        & .\tasks\winbuildscripts\pre-go-build.ps1
    }

    # MSI unit tests
    if ($Env:DEBUG_CUSTOMACTION) {
        & dda inv -- -e msi.test --debug
    } else {
        & dda inv -- -e msi.test
    }
    $err = $LASTEXITCODE
    Write-Host Test result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "Windows installer unit test failed $err"
        exit $err
    }

    # rtloader unit tests
    & dda inv -- -e rtloader.test
    $err = $LASTEXITCODE
    Write-Host rtloader test result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "rtloader test failed $err"
        exit $err
    }

    # Sanity check that the core agent can build
    & dda inv -- -e agent.build
    $err = $LASTEXITCODE
    if($err -ne 0){
        Write-Host -ForegroundColor Red "Agent build failed $err"
        exit $err
    }

    # Run python-script unit tests
    & dda inv -- -e invoke-unit-tests --directory=".\omnibus\python-scripts\"
    $err = $LASTEXITCODE
    Write-Host Python-script test result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "Python-script test failed $err"
        exit $err
    }

    # Run PowerShell install script unit tests
    & powershell -ExecutionPolicy Bypass -File ".\tools\windows\DatadogAgentInstallScript\Run-Tests.ps1"
    $err = $LASTEXITCODE
    Write-Host PowerShell install script test result is $err
    if($err -ne 0){
        Write-Host -ForegroundColor Red "PowerShell install script test failed $err"
        exit $err
    }

    # Go unit tests
    $test_output_file = if ($Env:TEST_OUTPUT_FILE) { $Env:TEST_OUTPUT_FILE } else { "test_output.json" }
    $TEST_WASHER_FLAG=""
    if ($Env:TEST_WASHER) {
        $TEST_WASHER_FLAG="--test-washer"
    }
    $Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3
    & dda inv -- -e test --junit-tar="$Env:JUNIT_TAR" `
        --race --profile --rerun-fails=2 --coverage --cpus 8 `
        --python-home-3=$Env:Python3_ROOT_DIR `
        --result-json C:\mnt\$test_output_file `
        --build-stdlib `
        $TEST_WASHER_FLAG `
        $Env:EXTRA_OPTS
    $err = $LASTEXITCODE

    if ($UploadCoverage) {
        # 1. Upload coverage reports to Codecov
        $Env:CODECOV_TOKEN=$(Get-VaultSecret -parameterName "$Env:CODECOV_TOKEN")
        & dda inv -- -e coverage.upload-to-codecov $Env:COVERAGE_CACHE_FLAG
        $localErr = $LASTEXITCODE
        if($localErr -ne 0){
            Write-Host -ForegroundColor Red "coverage upload failed $localErr"
        }
    }
    if ($UploadTestResults) {
        # 2. Upload junit files
        # Copy test files to c:\mnt for further gitlab upload
        Get-ChildItem -Filter "junit-out-*.xml" -Recurse | ForEach-Object {
            Copy-Item -Path $_.FullName -Destination C:\mnt
        }
        $Env:DATADOG_API_KEY=$(Get-VaultSecret -parameterName "$Env:API_KEY_ORG2")
        & dda inv -- -e junit-upload --tgz-path $Env:JUNIT_TAR --result-json C:\mnt\$test_output_file
        $localErr = $LASTEXITCODE
        if($localErr -ne 0){
            Write-Host -ForegroundColor Red "junit upload failed $localErr"
        }
    }

    If ($err -ne 0) {
        Write-Host -ForegroundColor Red "Go test failed $err"
        exit $err
    }

    Write-Host Test passed
}
