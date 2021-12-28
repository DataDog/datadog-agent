$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

Expand-Archive datadog-agent-latest.amd64.zip
Remove-Item datadog-agent-latest.amd64.zip

Get-ChildItem -Path datadog-agent-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

#   Install 7zip module
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Set-PSRepository -Name 'PSGallery' -SourceLocation "https://www.powershellgallery.com/api/v2" -InstallationPolicy Trusted
Install-Module -Name 7Zip4PowerShell -Force

#   Extract embbeded3.7z
Expand-7Zip -ArchiveFileName "C:/Program Files/Datadog/Datadog Agent/embedded3.7z" -TargetPath "C:/Program Files/Datadog/Datadog Agent"
Remove-Item "C:/Program Files/Datadog/Datadog Agent/embedded3.7z"

If (Test-Path -Path "C:/Program Files/Datadog/Datadog Agent/embedded2.7z") {
  Expand-7Zip -ArchiveFileName "C:/Program Files/Datadog/Datadog Agent/embedded2.7z" -TargetPath "C:/Program Files/Datadog/Datadog Agent"
  Remove-Item "C:/Program Files/Datadog/Datadog Agent/embedded2.7z"
}

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog'
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"
