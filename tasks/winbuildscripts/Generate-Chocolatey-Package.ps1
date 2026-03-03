<#
.SYNOPSIS
Generates a Chocolatey package for the Datadog Agent.

.PARAMETER msiDirectory
Specifies the directory containing the MSI file that will be used to calculate the checksum.

.PARAMETER Flavor
Specifies the flavor of the Datadog Agent. The default value is "datadog-agent".

.PARAMETER VersionOverride
Overrides the Agent version when building packages locally for testing.

.PARAMETER InstallDeps
Indicates whether to install dependencies. The default value is $true.

.EXAMPLE
.\Generate-Chocolatey-Package.ps1 -Flavor datadog-agent -VersionOverride "7.62.0" -msiDirectory C:\mnt\omnibus\pkg

Generates a chocolatey package for 7.62.0, requires the MSI file to be present in MSIDirectory.

.EXAMPLE
$env:CI_PIPELINE_ID="50910739"; .\Generate-Chocolatey-Package.ps1 -Flavor datadog-agent -VersionOverride "7.62.0-devel.git.276.e59b1b3.pipeline.50910739" -msiDirectory C:\mnt\omnibus\pkg

Generates a chocolatey package for PR/devel build 7.62.0-devel.git.276.e59b1b3.pipeline.50910739, requires the MSI file to be present in MSIDirectory.
The generated chocolatey package requires the MSI be uploaded to the dd-agent-mstesting bucket.
#>
Param(
    [Parameter(Mandatory=$true)]
    [String]
    $msiDirectory,

    [Parameter(Mandatory=$false)]
    [ValidateSet("datadog-agent", "datadog-fips-agent")]
    [String]
    $Flavor = "datadog-agent",

    [Parameter(Mandatory=$false)]
    [String]
    $VersionOverride,

    [bool] $InstallDeps = $true
)

$ErrorActionPreference = 'Stop';
Set-Location c:\mnt

if ($InstallDeps) {
    # Install chocolatey
    $env:chocolateyUseWindowsCompression = 'true'; Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))
    # Install dev tools, including invoke
    pip3 install dda
    dda self dep sync -f legacy-tasks
}

$repoRoot = "C:\mnt"
$outputDirectory = "$repoRoot\build-out"
if (![string]::IsNullOrEmpty($VersionOverride)) {
    $rawAgentVersion = $VersionOverride
} else {
    $rawAgentVersion = (dda inv -- agent.version --url-safe)
}
$copyright = "Datadog {0}" -f (Get-Date).Year

$releasePattern = "(\d+\.\d+\.\d+)"
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\.(\d+)"
$develPattern = "^(\d+\.\d+\.\d+)-devel\.git\.\d+\.(.+)"

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
    if ($Flavor -eq "datadog-agent") {
        # For historical reasons, use a different artifact name for the datadog-agent flavor
        # See agent-release-management for more details
        $artifactName = "ddagent-cli"
    } elseif ($Flavor -eq "datadog-fips-agent") {
        $artifactName = "datadog-fips-agent"
    } else {
        Write-Error "Unknown flavor $Flavor"
        exit 1
    }

    $packageSource = "$repoRoot\chocolatey\$Flavor"
    $nuspecFile = "$Flavor.nuspec"

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
        # For devel builds/branches, use the dd-agent-mstesting bucket URL
        # This allows us to build and test the package in PRs, and locally
        # by using the `-VersionOverride` param.
        if ([string]::IsNullOrEmpty($env:CI_PIPELINE_ID)) {
            Write-Error "CI_PIPELINE_ID is not set, aborting"
            exit 1
        } else {
            if ($rawAgentVersion -notmatch $env:CI_PIPELINE_ID) {
                Write-Error "CI_PIPELINE_ID is not found in the agent version, aborting" -ErrorAction Continue
                if ([string]::IsNullOrEmpty($env:BUCKET_BRANCH)) {
                    # dda inv agent.version requires BUCKET_BRANCH to be set when including pipeline in version
                    Write-Error "BUCKET_BRANCH is not set, if you are running this locally, set `$env:BUCKET_BRANCH='dev' or pass the -VersionOverride parameter" -ErrorAction Continue
                }
                exit 1
            }
            $url = "https://s3.amazonaws.com/dd-agent-mstesting/pipelines/A7/$env:CI_PIPELINE_ID/$flavor-$rawAgentVersion-1-x86_64.msi"
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
        exit 1
    }

    Write-Host "Generating Chocolatey package $flavor version $agentVersion in $(Get-Location)"

    # Template the install script with the URL and checksum
    try {
        $msiPath = Join-Path -Path "$msiDirectory" "$flavor-$rawAgentVersion-1-x86_64.msi"
        if (!(Test-Path $msiPath)) {
            Write-Host "Error: Could not find MSI file in $msiPath"
            Get-ChildItem "$msiDirectory"
            exit 1
        }
        $checksum = (Get-FileHash $msiPath -Algorithm SHA256).Hash
    }
    catch {
        Write-Host "Error: Could not generate checksum for package $($msiPath): $($_)"
        exit 1
    }
    # Set the $url in the install script
    (Get-Content $installScript).replace('$__url_from_ci__', '"' +  $url  + '"').replace('$__checksum_from_ci__', '"' +  $checksum  + '"') | Set-Content $installScript

    Write-Host "Generated nuspec file:"
    Write-Host (Get-Content $installScript | Out-String)

    if (!(Test-Path $outputDirectory)) {
        New-Item -ItemType Directory -Path $outputDirectory
    }
    Write-Host choco pack --out=$outputDirectory $nuspecFile --version $agentVersion release_notes=$releaseNotes copyright=$copyright
    choco pack --out=$outputDirectory $nuspecFile --version $agentVersion release_notes=$releaseNotes copyright=$copyright
} finally {
    Pop-Location
}
