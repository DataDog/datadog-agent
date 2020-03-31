$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$agentVersion=(inv agent.version) | Select-String -Pattern "\d+.\d+.\d+" | ForEach-Object{$_.Matches[0].Value}
Write-Host "Publishing Chocolatey package for agent version $agentVersion"
choco push .omnibus\pkg\datadog-agent.$agentVersion.nupkg -k $env:CHOCOLATEY_API_KEY --source https://push.chocolatey.org/