<#
    .SYNOPSIS
    Downloads and installs Datadog on the machine.
#>
[CmdletBinding(DefaultParameterSetName = 'Default')]
param(
    # The URL to download the installer from
    [Parameter(Mandatory = $false)]
    [string]
    $ddInstallerUrl = $env:DD_INSTALLER_URL
)

Write-Host "Welcome to the Datadog Install Script"
if (-Not $ddInstallerUrl) {
   # Replace with https://s3.amazonaws.com/ddagent-windows-stable/datadog-installer-x86_64.exe when ready
   $ddInstallerUrl = "https://s3.amazonaws.com/dd-agent-omnibus/datadog-installer-x86_64.exe"
}

$myWindowsID = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$myWindowsPrincipal = new-object System.Security.Principal.WindowsPrincipal($myWindowsID)
$adminRole = [System.Security.Principal.WindowsBuiltInRole]::Administrator
if ($myWindowsPrincipal.IsInRole($adminRole))
{
   # We are running "as Administrator"
   $Host.UI.RawUI.WindowTitle = $myInvocation.MyCommand.Definition + "(Elevated)"
   clear-host
}
else
{
   # We are not running "as Administrator
   $newProcess = new-object System.Diagnostics.ProcessStartInfo "PowerShell";
   $newProcess.Arguments = $myInvocation.MyCommand.Definition;
   $newProcess.Verb = "runas";
   [System.Diagnostics.Process]::Start($newProcess);
   exit
}

# Powershell does not enabled TLS 1.2 by default, & we want it enabled for faster downloads
Write-Host "Forcing web requests to TLS v1.2"
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

$installer = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-installer-x86_64.exe"
if (Test-Path $installer) {
   Remove-Item -Force $installer
}

Write-Host ("Downloading installer from {0}" -f $ddInstallerUrl)
[System.Net.WebClient]::new().DownloadFile($ddInstallerUrl, $installer)

# If not set the `default-packages` won't contain the Datadog Agent
$env:DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT="True"
Write-Host "Starting bootstrap process"
& $installer bootstrap
Write-Host "Bootstrap execution done"
Remove-Item -Force $installer
Write-Host "Datadog Install Script finished"
