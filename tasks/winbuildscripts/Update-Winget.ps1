$ErrorActionPreference = 'Stop';
Set-Location c:\mnt

# Install dev tools, including invoke
pip3 install -r requirements.txt

# Update the repo
$ghCliInstallResult = Start-Process "msiexec" -ArgumentList "/qn /i https://github.com/cli/cli/releases/download/v2.44.1/gh_2.44.1_windows_amd64.msi /log install.log" -NoNewWindow -Wait -Passthru
if ($ghCliInstallResult.ExitCode -ne 0) {
    Get-Content install.log | Write-Output
    Write-Error ("Failed to install Github CLI: {0}" -f $ghCliInstallResult.ExitCode)
} else {
    # Github CLI uses the GH_TOKEN
    $env:GH_TOKEN = $env:WINGET_GITHUB_ACCESS_TOKEN
    & 'C:\Program Files\GitHub CLI\gh.exe' repo sync https://github.com/robot-github-winget-datadog-agent/winget-pkgs.git --source microsoft/winget-pkgs
}

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
