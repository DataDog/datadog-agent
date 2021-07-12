$ErrorActionPreference = 'Stop';

$url = "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-$($env:chocolateyPackageVersion).msi"
# Note: match x.x.x-rc-x nuspec version format
$releaseCandidatePattern = "(\d+\.\d+\.\d+)-rc\-(\d+)"
if ($env:chocolateyPackageVersion -match $releaseCandidatePattern) {
  # and turn it back into Datadog version format x.x.x-rc.x
  $agentVersionMatches = $env:chocolateyPackageVersion | Select-String -Pattern $releaseCandidatePattern
  $url = "https://s3.amazonaws.com/dd-agent-mstesting/builds/tagged/datadog-agent-$($agentVersionMatches.Matches.Groups[1])-rc.$($agentVersionMatches.Matches.Groups[2])-1-x86_64.msi"
}

$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'msi'
  url64bit      = $url
  softwareName  = 'Datadog Agent'
  silentArgs    = "/qn /norestart /l*v `"$($env:TEMP)\$($packageName).$($env:chocolateyPackageVersion).MsiInstall.log`""
  validExitCodes= @(0, 3010, 1641)
}
Install-ChocolateyPackage @packageArgs

$installInfo = @"
---
install_method:
  tool: chocolatey
  tool_version: chocolatey-$($env:CHOCOLATEY_VERSION)
  installer_version: chocolatey_package-online
"@

$appDataDir = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Datadog\Datadog Agent").ConfigRoot
Out-File -FilePath $appDataDir\install_info -InputObject $installInfo
