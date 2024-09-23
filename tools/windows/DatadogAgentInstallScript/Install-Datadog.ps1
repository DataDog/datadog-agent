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
   $ddRemoteUpdates = $env:DD_REMOTE_UPDATES,

   # The minor version of the Agent to install, by default install the latest version
   [Parameter(Mandatory = $false)]
   [string]
   $ddAgentMinorVersion = $env:DD_AGENT_MINOR_VERSION
)

$SCRIPT_VERSION="1.0.0"
$SUCCESS=0
$GENERAL_ERROR_CODE=1

# ExitCodeException can be used to report failures from executables that set $LASTEXITCODE
class ExitCodeException : Exception {
    [string] $LastExitCode

    ExitCodeException($message, $lastExitCode) : base($message) {
        $this.LastExitCode = $lastExitCode
    }
}

function Update-ConfigFile($regex, $replacement) {
   $configFile = Join-Path (Get-ItemPropertyValue -Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent" -Name "ConfigRoot") "datadog.yaml"
   if (-Not $configFile) {
      $configFile = "C:\\ProgramData\\datadog.yaml"
   }
   if  (-Not (Test-Path $configFile)) {
      Write-Warning "datadog.yaml doesn't exist"
      return $GENERAL_ERROR_CODE
   }
   (Get-Content $configFile) -replace $regex, $replacement | Out-File configFile
   return $SUCCESS
}

function Report-Telemetry($payload) {
   $telemetryUrl = "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"
   if ($env:DD_SITE -eq "ddog-gov.com" -or -Not ($env:DD_API_KEY)) {
         return
   }

   if ($env:DD_SITE) {
      $telemetryUrl = ("https://instrumentation-telemetry-intake.${0}/api/v2/apmtelemetry" -f $env:DD_SITE)
   }

   Invoke-WebRequest -Uri $telemetryUrl -Method POST -Body $payload -ContentType "application/json" -Headers @{"DD-Api-Key"=$env:DD_API_KEY}
}

function Report-Error($errorMessage, $errorCode) {
    Write-Warning (@'
    Datadog Install script failed:

      {0}
      {1}

'@ -f $errorMessage, $errorCode)

    $agentVersion = "7.x"
    if ($ddAgentMinorVersion) {
        $agentVersion = ("7.{0}" -f $ddAgentMinorVersion)
    }
    Report-Telemetry (@'
{
   \"request_type\": \"apm-onboarding-event\",
   \"api_version\": \"v1\",
   \"payload\": {
       \"event_name\": \"agent.installation.error\",
       \"tags\": {
           \"install_id\": \"{0}\",
           \"install_type\": \"windows_powershell\",
           \"install_time\": \"{1}\"
           \"agent_platform\": \"windows\",
           \"agent_version\": \"{2}\",
           \"script_version\": \"{3}\"
       },
       \"error\": {
          \"code\": {4},
          \"message\": \"{5}\"
       }
   }
}
'@ -f (New-Guid).ToString(), [DateTimeOffset]::Now.ToUnixTimeSeconds(), $agentVersion, $SCRIPT_VERSION, $errorCode, ($errorMessage -replace '"', '_' -replace '\n', ' ' -replace '\r', ' '))
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

try {
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
   if ($LASTEXITCODE -ne 0)  {
      throw [ExitCodeException]::new("Bootstrap failed", $LASTEXITCODE)
   }
   Write-Host "Bootstrap execution done"

   if ($env:DD_API_KEY) {
      Write-Host "Writing DD_API_KEY"
      if (Update-ConfigFile("^[ #]*api_key:.*",  ("api_key: {0}" -f $env:DD_API_KEY)) -ne $SUCCESS) {
         throw "Writing DD_API_KEY failed"
      }
   }

   if ($env:DD_SITE) {
      Write-Host "Writing DD_SITE"
      if (Update-ConfigFile("^[ #]*site:.*",  ("site: {0}" -f $env:DD_SITE)) -ne $SUCCESS) {
         throw "Writing DD_SITE failed"
      }
   }

   if ($env:DD_URL) {
      Write-Host "Writing DD_URL"
      if (Update-ConfigFile("^[ #]*dd_url:.*",  ("dd_url: {0}" -f $env:DD_URL)) -ne $SUCCESS) {
         throw "Writing DD_URL failed"
      }
   }

   if ($ddRemoteUpdates) {
      Write-Host "Writing DD_REMOTE_UPDATES"
      if (Update-ConfigFile("^[ #]*remote_updates:.*",  ("remote_updates: {0}" -f $ddRemoteUpdates.ToLower())) -ne $SUCCESS) {
         throw "Writing DD_REMOTE_UPDATES failed"
      }
   }
   Report-Telemetry (@'
{
   \"request_type\": \"apm-onboarding-event\",
   \"api_version\": \"v1\",
   \"payload\": {
      \"event_name\": \"agent.installation.success\",
      \"tags\": {
         \"install_id\": \"{0}\",
         \"install_type\": \"windows_powershell\",
         \"install_time\": \"{1}\"
         \"agent_platform\": \"windows\",
         \"agent_version\": \"{2}\",
         \"script_version\": \"{3}\"
      }
   }
}
'@ -f (New-Guid).ToString(), [DateTimeOffset]::Now.ToUnixTimeSeconds(), $agentVersion, $SCRIPT_VERSION)

} catch [ExitCodeException] {
   Report-Error($_.Exception.Message, $_.Exception.LastExitCode)
} catch {
   Report-Error($_.Exception.Message, $GENERAL_ERROR_CODE)
} finally {
   Write-Host "Cleaning up..."
   Remove-Item -Force $installer
}
Write-Host "Datadog Install Script finished!"
