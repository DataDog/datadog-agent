# Fix Docker log file permissions for Datadog Agent
# This grants the ddagentuser read access to Docker log files

$dockerDir = "{{.DockerDir}}"
$containerPath = Join-Path $dockerDir "containers"

Write-Host "Granting ddagentuser read access to Docker logs..."
icacls "$containerPath" /grant ddagentuser:(OI)(CI)RX /T

Write-Host "Restarting Datadog Agent..."
Restart-Service -Name datadogagent -Force

Write-Host "Done! Check agent status with: & 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' status"
