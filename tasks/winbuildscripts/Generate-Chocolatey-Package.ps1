$ErrorActionPreference = 'Stop';
Set-Location c:\mnt
$outputDirectory = "c:\mnt\build-out"
$rawAgentVersion = (inv agent.version)

$releasePattern = "(\d+\.\d+\.\d+)"
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\.(\d+)"
$develPattern = "(\d+\.\d+\.\d+)-devel\+git\.\d+\.(.+)"

if ($rawAgentVersion -match $releaseCandidatePattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releaseCandidatePattern
    $agentVersion = "{0}-rc{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
} elseif ($rawAgentVersion -match $develPattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $develPattern
    $agentVersion = "{0}-devel{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
} elseif ($rawAgentVersion -match $releasePattern) {
    $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releasePattern
    $agentVersion = $agentVersionMatches.Matches.Groups[1].Value
}

Write-Host "Generating Chocolatey package version $agentVersion in $outputDirectory"

if (!(Test-Path $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory
}

choco pack --version=$agentVersion --out=$outputDirectory c:\mnt\chocolatey\datadog-agent.nuspec
