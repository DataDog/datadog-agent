$myWindowsID = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$myWindowsPrincipal = new-object System.Security.Principal.WindowsPrincipal($myWindowsID)
$adminRole = [System.Security.Principal.WindowsBuiltInRole]::Administrator
if ($myWindowsPrincipal.IsInRole($adminRole))
{
   # We are running "as Administrator"
   $Host.UI.RawUI.WindowTitle = $myInvocation.MyCommand.Definition + "(Elevated)"
   $Host.UI.RawUI.BackgroundColor = "DarkMagenta"
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

$installer = New-TemporaryFile
[System.Net.WebClient]::new().DownloadFile("https://s3.amazonaws.com/ddagent-windows-stable/datadog-installer.exe", $filePath)

# If not set the `default-packages` won't contain the Datadog Agent
$env:DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT="True"

& $installer bootstrap

Remove-Item -Force $installer
Write-Host "Done!"
