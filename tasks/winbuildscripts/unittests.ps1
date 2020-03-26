$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3
$Env:BUILD_ROOT=(Get-Location).Path
$Env:PATH="$Env:BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python2_ROOT_DIR;$Env:Python2_ROOT_DIR\Scripts;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

git clone --depth 1 https://github.com/datadog/integrations-core
& $Env:Python2_ROOT_DIR\python.exe -m pip install PyYAML==5.1
& $Env:Python3_ROOT_DIR\python.exe -m pip install PyYAML==5.1

$archflag = "x64"
if ($Env:TARGET_ARCH -eq "x86") {
    $archflag = "x86"
}
& go get gopkg.in/yaml.v2
& inv -e deps --verbose --dep-vendor-only

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

& inv -e test --race --profile --cpus 4 --arch $archflag --python-runtimes="$Env:PY_RUNTIMES" --python-home-2=$Env:Python2_ROOT_DIR --python-home-3=$Env:Python3_ROOT_DIR --rtloader-root=$Env:BUILD_ROOT\rtloader
$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "test failed $err"
    [Environment]::Exit($err)
}
