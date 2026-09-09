# Grant the ddagentuser account access to the Docker named pipe by adding it to the docker-users group.

Write-Host "Adding ddagentuser to the docker-users group..."
Add-LocalGroupMember -Group "docker-users" -Member "ddagentuser"

Write-Host "Restarting Datadog Agent..."
Restart-Service -Name datadogagent -Force

Write-Host "Done! Check agent status with: & 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' status"
