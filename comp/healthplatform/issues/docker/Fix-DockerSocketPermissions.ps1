# Grant the ddagentuser account access to the Docker named pipe.
#
# Docker's named pipe (docker_engine) only grants access to the local group
# configured via the daemon's "group" setting in daemon.json. Docker Desktop
# sets this to "docker-users" automatically; a standalone Docker Engine
# install on Windows Server does not configure any group by default, so
# adding ddagentuser to a group does nothing unless the daemon already
# trusts that group.

$daemonConfigPath = "$env:ProgramData\docker\config\daemon.json"
$targetGroup = "docker-users"
$daemonConfigured = $false

if (Test-Path $daemonConfigPath) {
    try {
        $daemonConfig = Get-Content $daemonConfigPath -Raw | ConvertFrom-Json
        if ($daemonConfig.group) {
            $targetGroup = $daemonConfig.group
            $daemonConfigured = $true
        }
    } catch {
        Write-Warning "Could not parse $daemonConfigPath, assuming no group is configured."
    }
}

if (-not (Get-LocalGroup -Name $targetGroup -ErrorAction SilentlyContinue)) {
    Write-Host "Creating local group '$targetGroup'..."
    New-LocalGroup -Name $targetGroup
}

Write-Host "Adding ddagentuser to '$targetGroup'..."
Add-LocalGroupMember -Group $targetGroup -Member "ddagentuser" -ErrorAction SilentlyContinue

if (-not $daemonConfigured) {
    Write-Warning "Docker's daemon has no 'group' configured in $daemonConfigPath, so membership in '$targetGroup' alone will NOT grant named-pipe access."
    Write-Warning "Add `"group`": `"$targetGroup`" to $daemonConfigPath and restart the Docker service (Restart-Service docker) for this to take effect."
    Write-Host "Skipping Datadog Agent restart until the Docker daemon is reconfigured."
    exit 1
}

Write-Host "Restarting Datadog Agent..."
Restart-Service -Name datadogagent -Force

Write-Host "Done! Check agent status with: & 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' status"
