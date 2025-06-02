$ErrorActionPreference = 'Stop';

$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'msi'
  # Note: Url is replaced at build time with the full URL to the MSI
  url64bit      = $__url_from_ci__
  checksum64    = $__checksum_from_ci__
  checksumType  = 'sha256'
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
