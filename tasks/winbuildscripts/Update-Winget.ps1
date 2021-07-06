$ErrorActionPreference = 'Stop';
Set-Location c:\mnt

Invoke-WebRequest https://aka.ms/wingetcreate/latest -OutFile wingetcreate.exe

# Install dev tools, including invoke
pip3 install -r requirements.txt

$rawAgentVersion = (inv agent.version)
$releasePattern = "(\d+\.\d+\.\d+)"
if ($rawAgentVersion -match $releasePattern) {
} else {
    Write-Host "Unsupported agent version '$rawAgentVersion', aborting"
    exit 1
}
Write-Host ("Updating Winget manifest for Agent version {0}" -f $rawAgentVersion)
wingetcreate.exe update --id "Datadog.Agent" --url "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-$($agentVersion).msi" --version $($agentVersion) --token $env:WINGET_GITHUB_ACCESS_TOKEN
