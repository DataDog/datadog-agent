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
& .\tasks\winbuildscripts\pre-go-build.ps1 -PythonRuntimes "$Env:PY_RUNTIMES"

& inv -e rtloader.test
$err = $LASTEXITCODE
Write-Host rtloader test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader test failed $err"
    [Environment]::Exit($err)
}

& inv -e install-tools
& inv -e test --junit-tar="$Env:JUNIT_TAR" --race --profile --rerun-fails=2 --coverage --cpus 8 --python-runtimes="$Env:PY_RUNTIMES" --python-home-2=$Env:Python2_ROOT_DIR --python-home-3=$Env:Python3_ROOT_DIR --save-result-json C:\mnt\$test_output_file $Env:EXTRA_OPTS --build-stdlib

$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "test failed $err"
    [Environment]::Exit($err)
}
