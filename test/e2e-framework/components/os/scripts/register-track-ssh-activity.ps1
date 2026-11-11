# Register a SYSTEM scheduled task running track-ssh-activity.ps1, which survives reboots:
# - when sshd logs an event, so shortly after an SSH connection opens
# - every 5 minutes, for long sessions
#
# Copied and run over SSH, see installWindowsSSHActivityTracking in scenarios/aws/ec2/os_win.go.
# Best effort: a failure prints a warning instead of failing the stack.

param(
  # Path of track-ssh-activity.ps1 on this host
  [Parameter(Mandatory = $true)][string]$ScriptPath
)

# sshd writes several events on every connection. They all share the same event ID, so the
# subscription cannot single out logins.
$sshEventSubscription = '<QueryList><Query Id="0" Path="OpenSSH/Operational"><Select Path="OpenSSH/Operational">*[System[EventID=4]]</Select></Query></QueryList>'

try {
  $action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File $ScriptPath"
  $timeTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 5)
  # New-ScheduledTaskTrigger does not support event triggers, they have to be built from the CIM class
  $eventTrigger = New-CimInstance -CimClass (Get-CimClass -Namespace 'Root/Microsoft/Windows/TaskScheduler' -ClassName 'MSFT_TaskEventTrigger') -ClientOnly
  $eventTrigger.Enabled = $true
  $eventTrigger.Subscription = $sshEventSubscription
  $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  $settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 2) -MultipleInstances IgnoreNew -StartWhenAvailable
  Register-ScheduledTask -TaskName 'dd-e2e-track-ssh-activity' -Action $action -Trigger @($timeTrigger, $eventTrigger) -Principal $principal -Settings $settings -Force | Out-Null
} catch {
  Write-Warning "Failed to register the SSH activity tracking task: $_"
}
