Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$PSDefaultParameterValues['*:ErrorAction'] = 'Stop'

$url = "https://curl.haxx.se/ca/cacert.pem"
$output = "$PSScriptRoot\cacert.pem"
$certlocation = "Cert:\LocalMachine\Root"

Write-Host "Downloading $url to $output " -ForegroundColor "Yellow"

[Net.ServicePointManager]::SecurityProtocol = "tls12, tls11, tls"
(New-Object System.Net.WebClient).DownloadFile($url, $output)

Write-Host "Converting to pfx with" -ForegroundColor "Yellow"
Write-Host "openssl pkcs12 -export -nokeys -out cacert.pfx -in cacert.pem" -ForegroundColor "White"
cmd /c "openssl pkcs12 -export -nokeys -out cacert.pfx -in cacert.pem -password pass:export"

Write-Host "Importing root certificates as" -ForegroundColor "Yellow"
Write-Host "Import-PfxCertificate –FilePath $PSScriptRoot\cacert.pfx Cert:\LocalMachine\Root" -ForegroundColor "White"
Import-PfxCertificate –FilePath $PSScriptRoot\cacert.pfx $certlocation -Password (ConvertTo-SecureString -String "export" -Force –AsPlainText)

Set-Location $certlocation
 
#Get the installed certificates in that location
Get-ChildItem | Format-Table Subject, FriendlyName, Thumbprint -AutoSize
