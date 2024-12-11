Param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("offline", "online")]
    [String]
    $installMethod,

    [Parameter(Mandatory=$false)]
    [String]
    $msiDirectory,

    [Parameter(Mandatory=$false)]
    [ValidateSet("datadog-agent", "datadog-fips-agent")]
    [String]
    $Flavor = "datadog-agent",

    [bool] $InstallDeps = $true
)

$ErrorActionPreference = 'Stop';
Set-Location c:\mnt

if ($InstallDeps) {
    # Install chocolatey
    $env:chocolateyUseWindowsCompression = 'true'; Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))
    # Install dev tools, including invoke
    pip3 install -r requirements.txt
}

$repoRoot = "C:\mnt"
$outputDirectory = "$repoRoot\build-out"
$rawAgentVersion = (inv agent.version --url-safe --major-version 7)
$copyright = "Datadog {0}" -f (Get-Date).Year

$releasePattern = "(\d+\.\d+\.\d+)"
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\.(\d+)"
$develPattern = "(\d+\.\d+\.\d+)-devel\.git\.\d+\.(.+)"

# Build the package in a temporary directory
# Some of the build steps modify the package source, so we don't want to do this in the source directory
$buildTempDir = [System.IO.Path]::GetTempPath() + "\datadog-choco-build"
if (Test-Path $buildTempDir) {
    Remove-Item -Recurse -Force $buildTempDir
}
New-Item -ItemType Directory -Path $buildTempDir | Out-Null
Push-Location -Path $buildTempDir
try {
    # Set the artifact name and package source based on the flavor
    if ($flavor -eq "datadog-agent") {
        # For historical reasons, use a different artifact name for the datadog-agent flavor
        # See agent-release-mangement for more details
        $artifactName = "ddagent-cli"
        $packageSource = "$repoRoot\chocolatey\datadog-agent\$installMethod"
        $nuspecFile = "datadog-agent-$installMethod.nuspec"
    } elseif ($flavor -eq "datadog-fips-agent") {
        if ($installMethod -eq "offline") {
            Write-Error "Offline install method not supported for flavor $flavor"
            exit 1
        }
        $artifactName = "datadog-fips-agent"
        $packageSource = "$repoRoot\chocolatey\datadog-fips-agent\online"
        $nuspecFile = "datadog-fips-agent-online.nuspec"
    } else {
        Write-Error "Unknown flavor $flavor"
        exit 1
    }

    # These files/directories are referenced in the nuspec file
    $licensePath = "tools\LICENSE.txt"
    $installScript = "tools\chocolateyinstall.ps1"

    # Copy package source to build temp dir
    Copy-Item -Recurse -Force -Path $packageSource\* -Destination $buildTempDir
    Copy-Item -Force -Path $repoRoot\LICENSE -Destination $licensePath

    # Generate URLs based on the Agent version
    if ($rawAgentVersion -match $releaseCandidatePattern) {
        $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releaseCandidatePattern
        $agentVersion = "{0}-rc-{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
        # We don't have release notes for RCs but this way the user can always see what commits are included in this RC
        $releaseNotes = "https://github.com/DataDog/datadog-agent/releases/tag/{0}-rc.{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2]
        $url = "https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/$artifactName-$($agentVersionMatches.Matches.Groups[1])-rc.$($agentVersionMatches.Matches.Groups[2]).msi"
    } elseif ($rawAgentVersion -match $develPattern) {
        if ($installMethod -eq "online") {
            # We don't publish online chocolatey packages for dev branches, error out
            Write-Host "Chocolatey packages are not built for dev branches aborting"
            exit 2
        }
        $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $develPattern
        $agentVersion = "{0}-devel-{1}" -f $agentVersionMatches.Matches.Groups[1], $agentVersionMatches.Matches.Groups[2].Value
        # We don't have release notes for devel, so point it to the generic url
        $releaseNotes = "https://github.com/DataDog/datadog-agent/releases"
    } elseif ($rawAgentVersion -match $releasePattern) {
        $agentVersionMatches = $rawAgentVersion | Select-String -Pattern $releasePattern
        $agentVersion = $agentVersionMatches.Matches.Groups[1].Value
        $releaseNotes = "https://github.com/DataDog/datadog-agent/releases/tag/$agentVersion"
        $url = "https://s3.amazonaws.com/ddagent-windows-stable/$artifactName-$($agentVersion).msi"
    } else {
        Write-Host "Unknown agent version '$rawAgentVersion', aborting"
        exit 3
    }

    Write-Host "Generating Chocolatey $installMethod package $flavor version $agentVersion in $(Get-Location)"

    # Template the install script with the URL and checksum
    if ($installMethod -eq "online") {
        try {
            $tempMsi = Join-Path -Path "$msiDirectory" "$flavor-$rawAgentVersion-1-x86_64.msi"
            if (!(Test-Path $tempMsi)) {
                Write-Host "Error: Could not find MSI file in $tempMsi"
                Get-ChildItem "$msiDirectory"
                exit 1
            }
            $checksum = (Get-FileHash $tempMsi -Algorithm SHA256).Hash
        }
        catch {
            Write-Host "Error: Could not generate checksum for package $($tempMsi): $($_)"
            exit 4
        }
        # Set the $url in the install script
        (Get-Content $installScript).replace('$__url_from_ci__', '"' +  $url  + '"').replace('$__checksum_from_ci__', '"' +  $checksum  + '"') | Set-Content $installScript
    }

    Write-Host "Generated nupsec file:"
    Write-Host (Get-Content $installScript | Out-String)

    if (!(Test-Path $outputDirectory)) {
        New-Item -ItemType Directory -Path $outputDirectory
    }
    Write-Host choco pack --out=$outputDirectory $nuspecFile --version $agentVersion release_notes=$releaseNotes copyright=$copyright
    choco pack --out=$outputDirectory $nuspecFile --version $agentVersion release_notes=$releaseNotes copyright=$copyright
} finally {
    Pop-Location
}
