$ErrorActionPreference = "Stop"
Trap { Write-Host "Error in install.ps1: $_" }

Expand-Archive datadog-agent-7-latest.amd64.zip
Remove-Item datadog-agent-7-latest.amd64.zip

Get-ChildItem -Path datadog-agent-7-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog' 
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"
