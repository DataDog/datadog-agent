$ErrorActionPreference = 'Stop';
# See https://docs.microsoft.com/en-us/windows/win32/cimwin32prov/win32-computersystem
$domainRole = (Get-WmiObject -Class Win32_ComputerSystem).DomainRole
if (($domainRole -eq 4) -Or ($domainRole -eq 5)) {
  Write-Host "Installation on a Domain Controller is not yet supported - aborting"
  exit -1
}
$toolsDir   = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$packageArgs = @{
  packageName   = $env:ChocolateyPackageName
  unzipLocation = $toolsDir
  fileType      = 'msi'
  url64bit      = "https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-$($env:chocolateyPackageVersion).msi"
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
  installer_version: chocolatey-$($env:chocolateyPackageVersion)-online
"@

Out-File -FilePath C:\ProgramData\Datadog\install_info -InputObject $installInfo
