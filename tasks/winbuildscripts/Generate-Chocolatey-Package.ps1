$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$outputDirectory = "c:\mnt\build-out"
$rawAgentVersion = (inv agent.version)
$copyright = "Datadog {0}" -f (Get-Date).Year

$releasePattern = "(\d+\.\d+\.\d+)"
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\.(\d+)"
$develPattern = "(\d+\.\d+\.\d+)-devel\+git\.\d+\.(.+)"

$installMethod = "online"
$nuspecFile = "c:\mnt\chocolatey\datadog-agent-online.nuspec"
$licensePath = "c:\mnt\chocolatey\tools-online\LICENSE.txt"
$msis = Get-ChildItem c:\mnt\chocolatey\datadog-agent*.msi
$msiCount = ($msis | Measure-Object).Count
if ($msiCount -eq 1) {
    $installMethod = "offline"
    $nuspecFile = "c:\mnt\chocolatey\datadog-agent-offline.nuspec"
    $licensePath = "c:\mnt\chocolatey\tools-offline\LICENSE.txt"
} elseif ($msiCount -gt 1) {
    Write-Host "Too many MSI found, aborting: '$msis'"
    exit 1
}

if ($rawAgentVersion -match $releaseCandidatePattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releaseCandidatePattern
    $agentVersion = "{0}-rc-{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
    # We don't have release notes for RCs but this way the user can always see what commits are included in this RC
    $releaseNotes = "https://github.com/DataDog/datadog-agent/releases/tag/{0}-rc.{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2]
} elseif ($rawAgentVersion -match $develPattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $develPattern
    $agentVersion = "{0}-devel-{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
    # We don't have release notes for devel, so point it to the generic url
    $releaseNotes = "https://github.com/DataDog/datadog-agent/releases"
} elseif ($rawAgentVersion -match $releasePattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releasePattern
    $agentVersion = $agentVersionMatches.Matches.Groups[1].Value
    $releaseNotes = "https://github.com/DataDog/datadog-agent/releases/tag/$agentVersion"
} else {
    Write-Host "Unknown agent version '$rawAgentVersion', aborting"
    exit 2
}

Invoke-WebRequest -Uri "https://raw.githubusercontent.com/DataDog/datadog-agent/master/LICENSE" -OutFile $licensePath

Write-Host "Generating Chocolatey $installMethod package version $agentVersion in $outputDirectory"

if (!(Test-Path $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory
}

choco pack --out=$outputDirectory $nuspecFile package_version=$agentVersion release_notes=$releaseNotes copyright=$copyright
