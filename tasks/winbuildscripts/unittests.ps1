$ErrorActionPreference = "Stop"
$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

$test_output_file = if ($Env:TEST_OUTPUT_FILE) { $Env:TEST_OUTPUT_FILE } else { "test_output.json" }

& ridk enable
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$UT_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$UT_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

& pip install -r tasks/libs/requirements-github.txt

mkdir  .\bin\agent

# Generate the datadog.yaml config file to be used in integration tests
& inv -e agent.generate-config --build-type="agent-py2py3" --output-file="./datadog.yaml"

# NG installer unit tests
if ($Env:DEBUG_CUSTOMACTION) {
    & inv -e msi.test --debug
} else {
    & inv -e msi.test
}
$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "Windows installer unit test failed $err"
    [Environment]::Exit($err)
}

& inv -e deps
& .\tasks\winbuildscripts\pre-go-build.ps1

& inv -e rtloader.test
$err = $LASTEXITCODE
Write-Host rtloader test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader test failed $err"
    [Environment]::Exit($err)
}
$TEST_WASHER_FLAG=""
if($Env:TEST_WASHER){
    $TEST_WASHER_FLAG="--test-washer"
}
& inv -e install-tools
& inv -e agent.build
$err = $LASTEXITCODE
if($err -ne 0){
    Write-Host -ForegroundColor Red "Agent build failed $err"
    [Environment]::Exit($err)
}
& inv -e test --junit-tar="$Env:JUNIT_TAR" --race --profile --rerun-fails=2 --coverage --cpus 8 --python-home-2=$Env:Python2_ROOT_DIR --python-home-3=$Env:Python3_ROOT_DIR --save-result-json C:\mnt\$test_output_file $Env:EXTRA_OPTS --build-stdlib $TEST_WASHER_FLAG
If ($LASTEXITCODE -ne "0") {
    exit $LASTEXITCODE
}

# Ignore upload failures
$ErrorActionPreference = "Continue"
$tmpfile = [System.IO.Path]::GetTempFileName()

# 1. Upload coverage reports to Codecov
& "$UT_BUILD_ROOT\tools\ci\fetch_secret.ps1" -parameterName "$Env:CODECOV_TOKEN" -tempFile "$tmpfile"
If ($LASTEXITCODE -ne "0") {
    Write-Host "Failed to fetch CODECOV_TOKEN - ignoring"
    exit "0"
}
$Env:CODECOV_TOKEN=$(cat "$tmpfile")
& inv -e coverage.upload-to-codecov $Env:COVERAGE_CACHE_FLAG
if($LASTEXITCODE -ne "0"){
    Write-Host -ForegroundColor Red "coverage upload failed $err"
}

# 2. Upload junit files
# Copy test files to c:\mnt for further gitlab upload
Get-ChildItem -Path "$UT_BUILD_ROOT" -Filter "junit-out-*.xml" -Recurse | ForEach-Object {
    Copy-Item -Path $_.FullName -Destination C:\mnt
}
& "$UT_BUILD_ROOT\tools\ci\fetch_secret.ps1" -parameterName "$Env:API_KEY_ORG2" -tempFile "$tmpfile"
If ($LASTEXITCODE -ne "0") {
    Write-Host "Failed to fetch API_KEY - ignoring"
    exit "0"
}
$Env:DATADOG_API_KEY=$(cat "$tmpfile")
Remove-Item "$tmpfile"

& inv -e junit-upload --tgz-path $Env:JUNIT_TAR
if($LASTEXITCODE -ne "0"){
    Write-Host -ForegroundColor Red "junit upload failed $err"
}

Write-Host Test passed
