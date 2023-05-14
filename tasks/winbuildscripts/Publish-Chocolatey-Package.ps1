$ErrorActionPreference = 'Stop';

# Install chocolatey binary
$env:chocolateyUseWindowsCompression = 'true'; Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))

Set-Location c:\mnt
$nupkgs = Get-ChildItem .\nupkg\datadog-agent*.nupkg
foreach($nupkg in $nupkgs) {
    Write-Host "Publishing Chocolatey package $($nupkg.Name) for agent version $agentVersion"
    choco push $nupkg.FullName --verbose --key $env:CHOCOLATEY_API_KEY --source https://push.chocolatey.org/
    If ($lastExitCode -ne "0") { throw "Previous command returned $lastExitCode" }
}
