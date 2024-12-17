﻿$ErrorActionPreference = 'Stop';

$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$nupkgs = Get-ChildItem $toolsDir\datadog-agent*.msi
if (($nupkgs | Measure-Object).Count -gt 1) {
  Write-Host "More than 1 MSI installer exists - aborting"
  exit -2
}
$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'msi'
  file          = $nupkgs[0].FullName
  softwareName  = 'Datadog Agent'
  silentArgs    = "/qn /norestart /l*v `"$($env:TEMP)\$($packageName).$($env:chocolateyPackageVersion).MsiInstall.log`""
  validExitCodes= @(0, 3010, 1641)
}
Install-ChocolateyInstallPackage @packageArgs

$installInfo = @"
---
install_method:
  tool: chocolatey
  tool_version: chocolatey-$($env:CHOCOLATEY_VERSION)
  installer_version: chocolatey_package-offline
"@

$appDataDir = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Datadog\Datadog Agent").ConfigRoot
Out-File -FilePath $appDataDir\install_info -InputObject $installInfo
