$ErrorActionPreference = 'Stop';
Set-Location c:\mnt

# Install dev tools, including invoke
pip3 install -r requirements.txt

$rawAgentVersion = (inv agent.version)
Write-Host "Detected agent version ${rawAgentVersion}"
$m = [regex]::match($rawAgentVersion, "(\d+\.\d+\.\d+)(-rc.(\d+))?")
if ($m) {
    $agentVersion = $m.Groups[1].Value
    $rc = $m.Groups[3].Value
} else {
    Write-Host "Unsupported agent version '$rawAgentVersion', aborting"
    exit 1
}
if ($rc) {
    Write-Host ("Updating Winget manifest for Agent version ${agentVersion}-rc.${rc}")
    wingetcreate.exe update --urls "https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/ddagent-cli-${agentVersion}-rc.${rc}.msi" --version "${agentVersion}-rc.${rc}" --submit --token "${env:WINGET_GITHUB_ACCESS_TOKEN}" "Datadog.Agent"
} else {
    Write-Host ("Updating Winget manifest for Agent version ${agentVersion}.1")
    wingetcreate.exe update --urls "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-${agentVersion}.msi" --version "${agentVersion}.1" --submit --token "${env:WINGET_GITHUB_ACCESS_TOKEN}" "Datadog.Agent"
}
if ($lastExitCode -ne 0) {
    Write-Error -Message "Wingetcreate update did not run successfully."
}
