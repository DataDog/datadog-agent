$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

if ($Env:TARGET_ARCH -eq "x64") {
    & ridk enable
}
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$Env:BUILD_ROOT=(Get-Location).Path
$Env:PATH="$Env:BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python2_ROOT_DIR;$Env:Python2_ROOT_DIR\Scripts;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

& $Env:Python3_ROOT_DIR\python.exe -m pip install PyYAML==5.3

& inv -e deps

& inv -e system-probe.kitchen-prepare

$err = $LASTEXITCODE
Write-Host Test result is $err
if($err -ne 0){
    Write-Host -ForegroundColor Red "kitchen prepare failed $err"
    [Environment]::Exit($err)
}
