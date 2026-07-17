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

Requires the CODECOV environment variable to be set.

.PARAMETER UploadTestResults
Specifies whether to upload test results to Datadog CI. Default is $false.

Requires the AGENT_API_KEY_ORG2 environment variable to be set.

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

trap {
    Write-Host "trap: $($_.InvocationInfo.Line.Trim()) - $_" -ForegroundColor Yellow
    continue
}

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
            if ([string]::IsNullOrEmpty($Env:CODECOV)) {
                Write-Host -ForegroundColor Red "CODECOV environment variable is required for uploading coverage reports to Codecov"
                exit 1
            }
        }
        if ($UploadTestResults) {
            if ([string]::IsNullOrEmpty($Env:AGENT_API_KEY_ORG2)) {
                Write-Host -ForegroundColor Red "AGENT_API_KEY_ORG2 environment variable is required for junit upload to Datadog CI"
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
        & dda inv -- -e schema.template --schema=./pkg/config/schema/yaml/core_schema.yaml --build-type=agent-py3 --os-target=windows --output=./datadog.yaml
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
        Show-BinaryHeaders -Path "$(bazel info bazel-bin 2>$null)\rtloader\install.exe"
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

    # Go unit tests
    # Compute a pipeline-specific filename to avoid leaving test_output.json inside C:\mnt
    # while Go tests run — a held-open file there blocks the next job's Git cleanup.
    $test_output_file = if ($Env:TEST_OUTPUT_FILE) { $Env:TEST_OUTPUT_FILE } else { "test_output.json" }
    $test_output_basename = [System.IO.Path]::GetFileNameWithoutExtension($test_output_file)
    $test_output_ext = [System.IO.Path]::GetExtension($test_output_file)
    if ([string]::IsNullOrEmpty($test_output_ext)) {
        $test_output_ext = ".json"
    }
    $pipeline_suffix = if ($Env:CI_PIPELINE_ID) { $Env:CI_PIPELINE_ID } else { "local" }
    $pipeline_test_output_file = "${test_output_basename}_${pipeline_suffix}${test_output_ext}"
    # When building out-of-source, write JSON to C:\buildroot so it is not held open inside C:\mnt
    # during test execution; otherwise write directly to C:\mnt.
    $mounted_result_json = "C:\mnt\$pipeline_test_output_file"
    $internal_result_json = if ($BuildOutOfSource) { "C:\buildroot\$pipeline_test_output_file" } else { $mounted_result_json }

    $TEST_WASHER_FLAG=""
    if ($Env:TEST_WASHER) {
        $TEST_WASHER_FLAG="--test-washer"
    }
    $Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3
    & dda inv -- -e test --junit-tar="$Env:JUNIT_TAR" `
        --race --profile --rerun-fails=2 --coverage --cpus 8 `
        --python-home-3=$Env:Python3_ROOT_DIR `
        --result-json $internal_result_json `
        --build-stdlib `
        $TEST_WASHER_FLAG `
        $Env:EXTRA_OPTS
    $err = $LASTEXITCODE

    if ($UploadCoverage) {
        try {
            $Env:CODECOV_TOKEN = Get-VaultSecret -parameterName "$Env:CODECOV" -parameterField token -ErrorAction Stop
            & dda inv -- -e coverage.upload-to-codecov $Env:COVERAGE_CACHE_FLAG
            if ($LASTEXITCODE -ne 0) {
                throw "coverage upload failed with exit code $LASTEXITCODE"
            }
        }
        catch {
            # Non-fatal: print but do not fail the script
            Write-Host -ForegroundColor Red "coverage upload failed (non-fatal): $($_.Exception.Message)"
        }
        # Upload coverage to Datadog Code Coverage (side-by-side with Codecov)
        try {
            $Env:DD_API_KEY = Get-VaultSecret -parameterName "$Env:AGENT_API_KEY_ORG2" -parameterField token -ErrorAction Stop
            & datadog-ci.exe coverage upload --format=go-coverprofile coverage.out
            if ($LASTEXITCODE -ne 0) {
                throw "Datadog coverage upload failed with exit code $LASTEXITCODE"
            }
        }
        catch {
            # Non-fatal: print but do not fail the script
            Write-Host -ForegroundColor Red "Datadog coverage upload failed (non-fatal): $($_.Exception.Message)"
        }
    }
    if ($UploadTestResults) {
        try {
            Get-ChildItem -Filter "junit-out-*.xml" -Recurse | ForEach-Object {
                Copy-Item -Path $_.FullName -Destination C:\mnt
            }
            $Env:DATADOG_API_KEY = Get-VaultSecret -parameterName "$Env:AGENT_API_KEY_ORG2" -parameterField token -ErrorAction Stop
            & dda inv -- -e junit-upload --tgz-path $Env:JUNIT_TAR --result-json $internal_result_json
            if($LASTEXITCODE -ne 0){
                throw "junit upload failed with exit code $LASTEXITCODE"
            }
        }
        catch {
            # Non-fatal: print but do not fail the script
            Write-Host -ForegroundColor Red "junit upload failed (non-fatal): $($_.Exception.Message)"
        }
    }

    if ($BuildOutOfSource) {
        # Copy the result JSON (and any unified variant produced by test-washer/junit-upload) back
        # into C:\mnt so GitLab can collect it as a pipeline artifact.
        if (Test-Path $internal_result_json) {
            Copy-Item -Force $internal_result_json $mounted_result_json
        }
        $internal_unified_json = "C:\buildroot\${test_output_basename}_${pipeline_suffix}_unified${test_output_ext}"
        $mounted_unified_json = "C:\mnt\${test_output_basename}_${pipeline_suffix}_unified${test_output_ext}"
        if (Test-Path $internal_unified_json) {
            Copy-Item -Force $internal_unified_json $mounted_unified_json
        }
    }

    If ($err -ne 0) {
        Write-Host -ForegroundColor Red "Go test failed $err"
        exit $err
    }

    Write-Host Test passed
}
