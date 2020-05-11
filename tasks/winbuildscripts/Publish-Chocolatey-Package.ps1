$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$nupkgs = Get-ChildItem .\nupkg\datadog-agent*.nupkg
foreach($nupkg in $nupkgs) {
    Write-Host "Publishing Chocolatey package $($nupkg.Name) for agent version $agentVersion"
    choco push $nupkg.FullName --verbose --key $env:CHOCOLATEY_API_KEY --source https://push.chocolatey.org/
}