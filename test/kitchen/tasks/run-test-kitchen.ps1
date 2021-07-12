if (Test-Path $PSScriptRoot\ssh-key) {
    Write-Host -ForegroundColor Yellow "Deleting existing $PSScriptRoot\ssh-key"
    Remove-Item $PSScriptRoot\ssh-key
}

if (Test-Path $PSScriptRoot\ssh-key.pub) {
    Write-Host -ForegroundColor Yellow "Deleting existing $PSScriptRoot\ssh-key"
    Remove-Item $PSScriptRoot\ssh-key.pub
}

$keyfile = "$PSScriptRoot\ssh-key"
Write-Host -ForegroundColor Green "Generating $PSScriptRoot\ssh-key"
& 'ssh-keygen.exe' -f $keyfile -P """" -t rsa -b 2048

$AZURE_SSH_KEY_PATH=$keyfile
& 'ssh-agent.exe' -s
& 'ssh-add' $keyfile

if (-not (Test-Path env:AZURE_CLIENT_ID)) {
    Write-Host -ForegroundColor Red "Need AZURE_CLIENT_ID"
    exit
}

if (-not (Test-Path env:AZURE_CLIENT_SECRET)) {
    Write-Host -ForegroundColor Red "Need AZURE_CLIENT_SECRET"
    exit
}
if (-not (Test-Path env:AZURE_TENANT_ID)) {
    Write-Host -ForegroundColor Red "Need AZURE_TENANT_ID"
    exit
}
if (-not (Test-Path env:AZURE_SUBSCRIPTION_ID)) {
    Write-Host -ForegroundColor Red "Need AZURE_SUBSCRIPTION_ID"
    exit
}
