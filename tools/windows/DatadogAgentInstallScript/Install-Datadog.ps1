<#
    .SYNOPSIS
    Downloads and installs Datadog on the machine.
#>
[CmdletBinding(DefaultParameterSetName = 'Default')]
param(
    # The URL where to download the installer
    [Parameter(Mandatory = $false)]
    [string]
    $ddInstallerUrl = $env:DD_INSTALLER_URL,

    # Whether or not to enable remote updates
    [Parameter(Mandatory = $false)]
    [string]
    $ddRemoteUpdates = $env:DD_REMOTE_UPDATES
)

function Update-ConfigFile($regex, $replacement) {
   $configFile = Join-Path (Get-ItemPropertyValue -Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent" -Name "ConfigRoot") "datadog.yaml"
   if (-Not $configFile) {
      $configFile = "C:\\ProgramData\\datadog.yaml"
   }
   if  (-Not (Test-Path $configFile)) {
      Write-Warning "datadog.yaml doesn't exist"
      return
   }
   (Get-Content $configFile) -replace $regex, $replacement | Out-File configFile
}

Write-Host "Welcome to the Datadog Install Script"

# Set some defaults if not provided
if (-Not $ddInstallerUrl) {
   # Replace with https://s3.amazonaws.com/ddagent-windows-stable/datadog-installer-x86_64.exe when ready
   $ddInstallerUrl = "https://s3.amazonaws.com/dd-agent-omnibus/datadog-installer-x86_64.exe"
}

if (-Not $ddRemoteUpdates) {
   $ddRemoteUpdates = "false"
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

if ($env:DD_API_KEY) {
   Write-Host "Writing DD_API_KEY"
   Update-ConfigFile("^[ #]*api_key:.*",  ("api_key: {0}" -f $env:DD_API_KEY))
}

if ($env:DD_SITE) {
   Write-Host "Writing DD_SITE"
   Update-ConfigFile("^[ #]*site:.*",  ("site: {0}" -f $env:DD_SITE))
}

if ($env:DD_URL) {
   Write-Host "Writing DD_URL"
   Update-ConfigFile("^[ #]*dd_url:.*",  ("dd_url: {0}" -f $env:DD_URL))
}

if ($ddRemoteUpdates) {
   Write-Host "Writing DD_REMOTE_UPDATES"
   Update-ConfigFile("^[ #]*remote_updates:.*",  ("remote_updates: {0}" -f $ddRemoteUpdates.ToLower()))
}

Write-Host "Cleaning up..."
Remove-Item -Force $installer

Write-Host "Datadog Install Script finished!"
