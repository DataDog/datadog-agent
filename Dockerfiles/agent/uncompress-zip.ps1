$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

Expand-Archive datadog-agent-latest.amd64.zip
Remove-Item datadog-agent-latest.amd64.zip

Get-ChildItem -Path datadog-agent-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

#   Install 7zip module
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$sevenzip="https://www.7-zip.org/a/7z1900-x64.exe"

Write-Host -ForegroundColor Green "Installing 7zip $sevenzip"
$out = "$($PSScriptRoot)\7zip.exe"
(New-Object System.Net.WebClient).DownloadFile($sevenzip, $out)
Start-Process 7zip.exe -ArgumentList '/S' -Wait
Remove-Item $out
setx PATH "$Env:Path;c:\program files\7-zip"
$Env:Path="$Env:Path;c:\program files\7-zip"
Write-Host -ForegroundColor Green Done with 7zip

#   Extract embbeded3.7z
& 7z x "C:/Program Files/Datadog/Datadog Agent/embedded3.7z" -o"C:/Program Files/Datadog/Datadog Agent"
Remove-Item "C:/Program Files/Datadog/Datadog Agent/embedded3.7z"

If (Test-Path -Path "C:/Program Files/Datadog/Datadog Agent/embedded2.7z") {
  & 7z x "C:/Program Files/Datadog/Datadog Agent/embedded2.7z" -o"C:/Program Files/Datadog/Datadog Agent"
  Remove-Item "C:/Program Files/Datadog/Datadog Agent/embedded2.7z"
}

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog'
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"
