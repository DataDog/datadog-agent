$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$nupkgs = Get-ChildItem .\build-out\datadog-agent*.nupkg
if (($nupkgs | Measure-Object).Count -gt 1) {
    Write-Host "More than 1 Chocolatey package exists - aborting"
    exit 1
}
Write-Host "Publishing Chocolatey package for agent version $agentVersion"
choco push $nupkgs[0].FullName -k $env:CHOCOLATEY_API_KEY --source https://push.chocolatey.org/
