$ErrorActionPreference = "Stop"
$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

& ridk enable
& $Env:Python3_ROOT_DIR\python.exe -m  pip install -r requirements.txt

$UT_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$UT_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

& inv -e deps
& .\tasks\winbuildscripts\pre-go-build.ps1

& inv -e install-tools
& inv -e integration-tests
$err = $LASTEXITCODE
if($err -ne 0){
    Write-Host -ForegroundColor Red "test failed $err"
    [Environment]::Exit($err)
}
Write-Host Test passed
