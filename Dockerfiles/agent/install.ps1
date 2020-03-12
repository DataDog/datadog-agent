$ErrorActionPreference = "Stop"

Expand-Archive datadog-agent-7-latest.amd64.zip
Remove-Item datadog-agent-7-latest.amd64.zip

Get-ChildItem -Path datadog-agent-7-* | Rename-Item -NewName "Datadog Agent"

New-Item -ItemType directory -Path "C:/Program Files/Datadog"
Move-Item "Datadog Agent" "C:/Program Files/Datadog/"

New-Item -ItemType directory -Path 'C:/ProgramData/Datadog' 
Move-Item "C:/Program Files/Datadog/Datadog Agent/EXAMPLECONFSLOCATION" "C:/ProgramData/Datadog/conf.d"

# Register as a service
New-Service -Name "datadogagent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe"
New-Service -Name "datadog-process-agent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe"
New-Service -Name "datadog-trace-agent" -StartupType "Manual" -BinaryPathName "C:\Program Files\Datadog\Datadog Agent\bin\agent\trace-agent.exe"
