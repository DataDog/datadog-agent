$ErrorActionPreference = "Stop"
$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

if ($Env:TARGET_ARCH -eq "x64") {
    & ridk enable
}
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

# Run invoke tasks unit tests
& $Env:Python3_ROOT_DIR\python.exe -m tasks.release_tests
& $Env:Python3_ROOT_DIR\python.exe -m tasks.libs.version_tests

$Env:BUILD_ROOT=(Get-Location).Path
$Env:PATH="$Env:BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

& $Env:Python3_ROOT_DIR\python.exe -m pip install PyYAML==5.3.1

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

& $Env:BUILD_ROOT\bin\agent\customaction-tests.exe
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

& inv -e rtloader.make --python-runtimes="$Env:PY_RUNTIMES" --install-prefix=$Env:BUILD_ROOT\dev --cmake-options='-G \"Unix Makefiles\"' --arch $archflag
$err = $LASTEXITCODE
Write-Host Build result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader make failed $err"
    [Environment]::Exit($err)
}

& inv -e rtloader.install
$err = $LASTEXITCODE
Write-Host rtloader install result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader install failed $err"
    [Environment]::Exit($err)
}

# & inv -e rtloader.format --raise-if-changed
# $err = $LASTEXITCODE
# Write-Host Format result is $err

# if($err -ne 0){
#   Write-Host -ForegroundColor Red "rtloader format failed $err"
#   [Environment]::Exit($err)
# }

& inv -e rtloader.test
$err = $LASTEXITCODE
Write-Host rtloader test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "rtloader test failed $err"
    [Environment]::Exit($err)
}

& inv -e install-tools
& inv -e test --junit-tar="$Env:JUNIT_TAR" --race --profile --rerun-fails=2 --cpus 4 --arch $archflag --python-runtimes="$Env:PY_RUNTIMES" --python-home-2=$Env:Python2_ROOT_DIR --python-home-3=$Env:Python3_ROOT_DIR --save-result-json C:\mnt\test_output.json

$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "test failed $err"
    [Environment]::Exit($err)
}
