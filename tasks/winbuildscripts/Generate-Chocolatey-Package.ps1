$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$outputDirectory = "c:\mnt\build-out"
$packageId = "datadog-agent"
# Specifying a custom PACKAGE_ID is useful for testing (i.e. publishing under a different ID, say test-datadog-agent)
if ($env:PACKAGE_ID) {
    $packageId = $env:PACKAGE_ID
}
$rawAgentVersion = (inv agent.version)
$copyright = "Datadog {0}" -f (Get-Date).Year

$releasePattern = "(\d+\.\d+\.\d+)"
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\.(\d+)"
$develPattern = "(\d+\.\d+\.\d+)-devel\+git\.\d+\.(.+)"

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
    exit -42
}

Write-Host "Generating Chocolatey package version $agentVersion in $outputDirectory"

if (!(Test-Path $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory
}

choco pack --out=$outputDirectory c:\mnt\chocolatey\datadog-agent.nuspec package_id=$packageId package_version=$agentVersion release_notes=$releaseNotes copyright=$copyright
if ($env:PACKAGE_ID) {
    # Changing the package id changes the name of the nupkg, so rename it here
    Move-Item "$outputDirectory\$packageId.$agentVersion.nupkg" "$outputDirectory\datadog-agent.$agentVersion.nupkg" 
}