$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python2_ROOT_DIR=$Env:TEST_EMBEDDED_PY2
$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

& ridk enable
& dda self dep sync -f legacy-tasks

$PROBE_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$PROBE_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:Python2_ROOT_DIR;$Env:Python2_ROOT_DIR\Scripts;$Env:PATH"

& dda inv -- -e install-tools

& dda inv -- -e system-probe.e2e-prepare --ci

$err = $LASTEXITCODE
if($err -ne 0){
    Write-Host -ForegroundColor Red "e2e prepare failed $err"
    [Environment]::Exit($err)
}
Write-Host Test passed
