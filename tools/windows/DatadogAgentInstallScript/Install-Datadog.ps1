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

$SCRIPT_VERSION = "1.0.0"
$SUCCESS = 0
$GENERAL_ERROR_CODE = 1

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
   if (-Not (Test-Path $configFile)) {
      throw "datadog.yaml doesn't exist"
   }
   (Get-Content $configFile) -replace $regex, $replacement | Out-File configFile
}

function Send-Telemetry($payload) {
   $telemetryUrl = "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"
   if ($env:DD_SITE -eq "ddog-gov.com" -or -Not ($env:DD_API_KEY)) {
      return
   }

   if ($env:DD_SITE) {
      $telemetryUrl = "https://instrumentation-telemetry-intake.${env:DD_SITE}/api/v2/apmtelemetry"
   }
   $requestHeaders = @{
      "DD-Api-Key"   = $env:DD_API_KEY
      "Content-Type" = "application/json"
   }
   $result = Invoke-WebRequest -Uri $telemetryUrl -Method POST -Body $payload -Headers $requestHeaders
   Write-Host "Sending telemetry: $($result.StatusCode)"
}

function Show-Error($errorMessage, $errorCode) {
   # Report as warning for prettier output and not having to deal with script termination
   Write-Warning @"
    Datadog Install script failed:

    Error message: $($errorMessage)
    Error code: $($errorCode)

"@

   $agentVersion = "7.x"
   if ($ddAgentMinorVersion) {
      $agentVersion = "7.${ddAgentMinorVersion}"
   }
   $errorMessage = ($errorMessage -replace '"', '_' -replace '\n', ' ' -replace '\r', ' ')

   Send-Telemetry @"
{
   "request_type": "apm-onboarding-event",
   "api_version": "v1",
   "payload": {
       "event_name": "agent.installation.error",
       "tags": {
           "install_id": "$(New-Guid)",
           "install_type": "windows_powershell",
           "install_time": "$([DateTimeOffset]::Now.ToUnixTimeSeconds())"
           "agent_platform": "windows",
           "agent_version: "$($agentVersion)",
           "script_version": "$($SCRIPT_VERSION)"
       },
       "error": {
          "code": "$($errorCode)",
          "message": "$($errorMessage)"
       }
   }
}
"@
}

function Start-ProcessWithOutput {
   param ([string]$Path, [string[]]$ArgumentList)
   $psi = New-object System.Diagnostics.ProcessStartInfo 
   $psi.CreateNoWindow = $true 
   $psi.UseShellExecute = $false 
   $psi.RedirectStandardOutput = $true 
   $psi.RedirectStandardError = $true 
   $psi.FileName = $Path
   if ($ArgumentList.Count -gt 0) {
      $psi.Arguments = $ArgumentList
   }
   $process = New-Object System.Diagnostics.Process 
   $process.StartInfo = $psi
   $stdout = Register-ObjectEvent -InputObject $process -EventName 'OutputDataReceived'`
      -Action {
      if (![String]::IsNullOrEmpty($EventArgs.Data)) {
         Write-Host $EventArgs.Data
      }
   }
   $stderr = Register-ObjectEvent -InputObject $process -EventName 'ErrorDataReceived' `
      -Action {
      if (![String]::IsNullOrEmpty($EventArgs.Data)) {
         Write-Warning $EventArgs.Data
      }
   }
   [void]$process.Start()
   $process.BeginOutputReadLine()
   $process.BeginErrorReadLine()
   $process.WaitForExit()
   Unregister-Event -SourceIdentifier $stdout.Name
   Unregister-Event -SourceIdentifier $stderr.Name
   return $process.ExitCode
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
if ($myWindowsPrincipal.IsInRole($adminRole)) {
   # We are running "as Administrator"
   $Host.UI.RawUI.WindowTitle = $myInvocation.MyCommand.Definition + "(Elevated)"
}
else {
   # We are not running "as Administrator
   $newProcess = new-object System.Diagnostics.ProcessStartInfo "PowerShell";
   $newProcess.Arguments = $myInvocation.MyCommand.Definition;
   $newProcess.Verb = "runas";
   [System.Diagnostics.Process]::Start($newProcess);
   exit
}

try {
   # Powershell does not enable TLS 1.2 by default, & we want it enabled for faster downloads
   Write-Host "Forcing web requests to TLS v1.2"
   [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

   $installer = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-installer-x86_64.exe"
   if (Test-Path $installer) {
      Remove-Item -Force $installer
   }

   Write-Host ("Downloading installer from {0}" -f $ddInstallerUrl)
   [System.Net.WebClient]::new().DownloadFile($ddInstallerUrl, $installer)

   # If not set the `default-packages` won't contain the Datadog Agent
   $env:DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT = "True"

   Write-Host "Starting bootstrap process"
   $result = Start-ProcessWithOutput -Path $installer -ArgumentList "bootstrap"
   if ($result -ne 0) {
      # bootstrap only fails if it fails to install to install the Datadog Installer, so it's possible the Agent was not  installed
      throw [ExitCodeException]::new("Bootstrap failed", $result)
   }
   Write-Host "Bootstrap execution done"

   if (-Not (Test-Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent")) {
      throw "Agent is not installed"
   }

   if ($env:DD_API_KEY) {
      Write-Host "Writing DD_API_KEY"
      if (Update-ConfigFile "^[ #]*api_key:.*" ("api_key: {0}" -f $env:DD_API_KEY) -ne $SUCCESS) {
         throw "Writing DD_API_KEY failed"
      }
   }

   if ($env:DD_SITE) {
      Write-Host "Writing DD_SITE"
      if (Update-ConfigFile "^[ #]*site:.*" ("site: {0}" -f $env:DD_SITE) -ne $SUCCESS) {
         throw "Writing DD_SITE failed"
      }
   }

   if ($env:DD_URL) {
      Write-Host "Writing DD_URL"
      if (Update-ConfigFile "^[ #]*dd_url:.*" ("dd_url: {0}" -f $env:DD_URL) -ne $SUCCESS) {
         throw "Writing DD_URL failed"
      }
   }

   if ($ddRemoteUpdates) {
      Write-Host "Writing DD_REMOTE_UPDATES"
      if (Update-ConfigFile "^[ #]*remote_updates:.*" ("remote_updates: {0}" -f $ddRemoteUpdates.ToLower()) -ne $SUCCESS) {
         throw "Writing DD_REMOTE_UPDATES failed"
      }
   }
   Send-Telemetry @"
{
   "request_type": "apm-onboarding-event",
   "api_version": "v1",
   "payload": {
       "event_name": "agent.installation.success",
       "tags": {
           "install_id": "$(New-Guid)",
           "install_type": "windows_powershell",
           "install_time": "$([DateTimeOffset]::Now.ToUnixTimeSeconds())"
           "agent_platform": "windows",
           "agent_version: "$($agentVersion)",
           "script_version": "$($SCRIPT_VERSION)"
       }
   }
}
"@

}
catch [ExitCodeException] {
   Show-Error $_.Exception.Message $_.Exception.LastExitCode
}
catch {
   Show-Error $_.Exception.Message $GENERAL_ERROR_CODE
}
finally {
   Write-Host "Cleaning up..."
   Remove-Item -Force $installer
}
Write-Host "Datadog Install Script finished!"
