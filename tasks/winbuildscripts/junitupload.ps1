$ErrorActionPreference = "Stop"
$Password = ConvertTo-SecureString "dummyPW_:-gch6Rejae9" -AsPlainText -Force
New-LocalUser -Name "ddagentuser" -Description "Test user for the secrets feature on windows." -Password $Password

$Env:Python3_ROOT_DIR=$Env:TEST_EMBEDDED_PY3

if ($Env:TARGET_ARCH -eq "x64") {
    & ridk enable
}
& $Env:Python3_ROOT_DIR\python.exe -m pip install -r c:\mnt\requirements.txt

$UT_BUILD_ROOT=(Get-Location).Path
$Env:PATH="$UT_BUILD_ROOT\dev\lib;$Env:GOPATH\bin;$Env:Python3_ROOT_DIR;$Env:Python3_ROOT_DIR\Scripts;$Env:PATH"

$url = 'https://github.com/DataDog/datadog-ci/releases/latest/download/datadog-ci_win-x64.exe'
(New-Object System.Net.WebClient).DownloadFile($url, "datadog-ci.exe")
$DebugPreference = 'SilentlyContinue'
& $Env:DATADOG_API_KEY = (aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.datadog_api_key_org2 --with-decryption --query "Parameter.Value" --out text)
$DebugPreference = 'Continue'
Get-ChildItem -Filter 'junit-*.tgz' | ForEach-Object { inv -e junit-upload --tgz-path $_.FullName }

if ($LASTEXITCODE -ne 0) {
    Write-Host "[Error]: Some unit tests failed"
    exit $LASTEXITCODE
}
