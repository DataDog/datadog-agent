$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
if (Get-ChildItem .\build-out\datadog-agent*.nupkg | Measure-Object).Count > 1) {
    Write-Host "More than 1 Chocolatey package exists - aborting"
}
Write-Host "Publishing Chocolatey package for agent version $agentVersion"
choco push .omnibus\pkg\datadog-agent*.nupkg -k $env:CHOCOLATEY_API_KEY --source https://push.chocolatey.org/