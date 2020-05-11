$ErrorActionPreference = 'Stop';
# See https://docs.microsoft.com/en-us/windows/win32/cimwin32prov/win32-computersystem
$domainRole = (Get-WmiObject -Class Win32_ComputerSystem).DomainRole
if (($domainRole -eq 4) -Or ($domainRole -eq 5)) {
  Write-Host "Installation on a Domain Controller is not yet supported - aborting"
  exit -1 
}
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
