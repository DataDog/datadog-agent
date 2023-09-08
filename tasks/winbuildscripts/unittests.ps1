$ErrorActionPreference = "Stop"
$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

if ($Env:TARGET_ARCH -eq "x64") {
    & ridk enable
}
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$UT_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$UT_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

& $Env:Python3_ROOT_DIR\python.exe -m pip install PyYAML==5.3.1

& inv -e invoke-unit-tests

if ($LASTEXITCODE -ne 0) {
    Write-Host "[Error]: Some unit tests failed"
    exit $LASTEXITCODE
}

& pushd "test\kitchen"

& inv -e kitchen.invoke-unit-tests

if ($LASTEXITCODE -ne 0) {
    Write-Host "[Error]: Some kitchen unit tests failed"
    exit $LASTEXITCODE
}

& popd

$archflag = "x64"
if ($Env:TARGET_ARCH -eq "x86") {
    $archflag = "x86"
}

mkdir  .\bin\agent
if ($Env:DEBUG_CUSTOMACTION) {
    & inv -e customaction.build --arch=$archflag --debug
} else {
    & inv -e customaction.build --arch=$archflag
}

# Generate the datadog.yaml config file to be used in integration tests
& inv -e generate-config --build-type="agent-py2py3" --output-file="./datadog.yaml"

& $UT_BUILD_ROOT\bin\agent\customaction-tests.exe
$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "custom action test failed $err"
    [Environment]::Exit($err)
}

# NG installer unit tests
if ($Env:DEBUG_CUSTOMACTION) {
    & inv -e msi.test --arch=$archflag --debug
} else {
    & inv -e msi.test --arch=$archflag
}
$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "Windows installer unit test failed $err"
    [Environment]::Exit($err)
}

& inv -e deps
& .\tasks\winbuildscripts\pre-go-build.ps1 -Architecture "$archflag" -PythonRuntimes "$Env:PY_RUNTIMES"

& inv -e rtloader.test
$err = $LASTEXITCODE
Write-Host rtloader test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader test failed $err"
    [Environment]::Exit($err)
}

& inv -e install-tools
& inv -e test --skip-linters --junit-tar="$Env:JUNIT_TAR" --race --profile --rerun-fails=2 --cpus 8 --arch $archflag --python-runtimes="$Env:PY_RUNTIMES" --python-home-2=$Env:Python2_ROOT_DIR --python-home-3=$Env:Python3_ROOT_DIR --save-result-json C:\mnt\test_output.json

$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "test failed $err"
    [Environment]::Exit($err)
}
