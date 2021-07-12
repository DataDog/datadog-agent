
# Set a fallback empty config with no AD
# Don't override /etc/datadog-agent/datadog.yaml if it exists
if (-not (Test-Path C:\ProgramData\Datadog\datadog.yaml)) { 
    Write-Output "Autodiscovery not enabled"
    Write-Output $null >> C:\ProgramData\Datadog\datadog.yaml
}
