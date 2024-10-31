<#
   .SYNOPSIS
   Downloads and installs Datadog on the machine.
#>
[CmdletBinding(DefaultParameterSetName = 'Default')]
$SCRIPT_VERSION = "1.0.0"
$GENERAL_ERROR_CODE = 1

# Set some defaults if not provided
$ddInstallerUrl = $env:DD_INSTALLER_URL
if (-Not $ddInstallerUrl) {
   $ddInstallerUrl = "https://s3.amazonaws.com/dd-agent-mstesting/datadog-installer-x86_64.exe"
}

$ddRemoteUpdates = $env:DD_REMOTE_UPDATES
if (-Not $ddRemoteUpdates) {
   $ddRemoteUpdates = "false"
}

# ExitCodeException can be used to report failures from executables that set $LASTEXITCODE
class ExitCodeException : Exception {
   [string] $LastExitCode

   ExitCodeException($message, $lastExitCode) : base($message) {
      $this.LastExitCode = $lastExitCode
   }
}

function Get-DatadogConfigPath() {
   if (
      (Test-Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent") -and
      ($null -ne (Get-Item -Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent").GetValue("ConfigRoot"))
   ) {
      return (Join-Path (Get-ItemPropertyValue -Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent" -Name "ConfigRoot") "datadog.yaml")
   }
   return "C:\\ProgramData\\Datadog\\datadog.yaml"
}

function Update-DatadogConfigFile($regex, $replacement) {
   $configFile = Get-DatadogConfigPath
   if (-Not (Test-Path $configFile)) {
      throw "datadog.yaml doesn't exist"
   }
   if (((Get-Content $configFile) | Select-String $regex | Measure-Object).Count -eq 0) {
      Add-Content -Path $configFile -Value $replacement
   }
   else {
    (Get-Content $configFile) -replace $regex, $replacement | Out-File $configFile
   }
}

function Send-Telemetry($payload) {
   $telemetryUrl = "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"
   if ($env:DD_SITE -eq "ddog-gov.com" -or -Not ($env:DD_API_KEY)) {
      return
   }

   if ($env:DD_SITE) {
      $telemetryUrl = "https://instrumentation-telemetry-intake.$env:DD_SITE/api/v2/apmtelemetry"
   }
   $requestHeaders = @{
      "DD-Api-Key"   = $env:DD_API_KEY
      "Content-Type" = "application/json"
   }
   try {
      $result = Invoke-WebRequest -Uri $telemetryUrl -Method POST -Body $payload -Headers $requestHeaders
      Write-Host "Sending telemetry: $($result.StatusCode)"
   } catch {
      # Don't propagate errors when sending telemetry, because our error handling code will also
      # try to send telemetry.
      # It's enough to just print a message since there's no further error handling to be done.
      Write-Host "Error sending telemetry"
   }
}

function Show-Error($errorMessage, $errorCode) {
   Write-Error -ErrorAction Continue @"
    Datadog Install script failed:

    Error message: $($errorMessage)
    Error code: $($errorCode)

"@

   $agentVersion = "7.x"
   if ($env:DD_AGENT_MINOR_VERSION) {
      $agentVersion = "7.$env:DD_AGENT_MINOR_VERSION"
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
         # Print stderr from process into host stderr
         # Unfortunately that means this output cannot be captured from within PowerShell
         # and it won't work within PowerShell ISE because it is not a console host.
         [Console]::ForegroundColor = 'red'
         [Console]::Error.WriteLine($EventArgs.Data)
         [Console]::ResetColor()
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

function Test-DatadogAgentPresence() {
   # Rudimentary check for the Agent presence, the `datadogagent` service should exist, and so should the `InstallPath` key in the registry.
   # We check that particular key since we use it later in the script to restart the service.
   return ( 
      ((Get-Service "datadogagent" -ea silent | Measure-Object).Count -eq 1) -and
      (Test-Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent") -and
      ($null -ne (Get-Item -Path "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent").GetValue("InstallPath"))
   )
}

function Update-DatadogAgentConfig() {
   if ($env:DD_API_KEY) {
      Write-Host "Writing DD_API_KEY"
      Update-DatadogConfigFile "^[ #]*api_key:.*" "api_key: $env:DD_API_KEY"
   }

   if ($env:DD_SITE) {
      Write-Host "Writing DD_SITE"
      Update-DatadogConfigFile "^[ #]*site:.*" "site: $env:DD_SITE"
   }

   if ($env:DD_URL) {
      Write-Host "Writing DD_URL"
      Update-DatadogConfigFile "^[ #]*dd_url:.*" "dd_url: $env:DD_URL"
   }

   if ($ddRemoteUpdates) {
      Write-Host "Writing DD_REMOTE_UPDATES"
      Update-DatadogConfigFile "^[ #]*remote_updates:.*" "remote_updates: $($ddRemoteUpdates.ToLower())"
   }
}

try {
   Write-Host "Welcome to the Datadog Install Script"
   if (-not [Environment]::Is64BitProcess) {
      throw "This command must be run in a 64-bit environment."
   }

   $myWindowsID = [System.Security.Principal.WindowsIdentity]::GetCurrent()
   $myWindowsPrincipal = new-object System.Security.Principal.WindowsPrincipal($myWindowsID)
   $adminRole = [System.Security.Principal.WindowsBuiltInRole]::Administrator
   if (-not $myWindowsPrincipal.IsInRole($adminRole)) {
      # We are not running "as Administrator"
      $newProcess = new-object System.Diagnostics.ProcessStartInfo "PowerShell";
      $newProcess.Arguments = $myInvocation.MyCommand.Definition;
      $newProcess.Verb = "runas";
      $proc = [System.Diagnostics.Process]::Start($newProcess);
      $proc.WaitForExit()
      return $proc.ExitCode
   }

   # First thing to do is to stop the services if they are started
   if (Test-DatadogAgentPresence) {
      Write-Host "Stopping Datadog Agent services"
      & ((Get-ItemProperty "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent").InstallPath + "bin\\agent.exe") stop-service
   }

   if ((Get-Service "Datadog Installer" -ea silent | Measure-Object).Count -eq 1) {
      Write-Host "Stopping Datadog Installer service"
      Stop-Service "Datadog Installer"
   }

   $configUpdated = $False
   # Write the config before-hand if it exists, that way if the Agent/Installer services start
   # once installed, they will have a valid configuration.
   # This allows the MSI to emit some telemetry as well.
   if (Test-Path (Get-DatadogConfigPath)) {
      Update-DatadogAgentConfig
      $configUpdated = $True
   }

   # Powershell does not enable TLS 1.2 by default, & we want it enabled for faster downloads
   Write-Host "Forcing web requests to TLS v1.2"
   [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

   $installer = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-installer-x86_64.exe"
   if (Test-Path $installer) {
      Remove-Item -Force $installer
   }

   Write-Host "Downloading installer from $ddInstallerUrl"
   [System.Net.WebClient]::new().DownloadFile($ddInstallerUrl, $installer)

   # If not set the `default-packages` won't contain the Datadog Agent
   $env:DD_INSTALLER_DEFAULT_PKG_INSTALL_DATADOG_AGENT = "True"

   Write-Host "Starting bootstrap process"
   $result = Start-ProcessWithOutput -Path $installer -ArgumentList "bootstrap"
   if ($result -ne 0) {
      # bootstrap only fails if it fails to install to install the Datadog Installer, so it's possible the Agent was not installed
      throw [ExitCodeException]::new("Bootstrap failed", $result)
   }
   Write-Host "Bootstrap execution done"

   if (-Not (Test-DatadogAgentPresence)) {
      throw "Agent is not installed"
   }

   if (-Not ($configUpdated)) {
      Update-DatadogAgentConfig
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
   # The datadog.yaml configuration was potentially modified so restart the services
   Write-Host "Starting Datadog Installer service"
   Restart-Service "Datadog Installer"
   # This command handles restarting the dependent services as well
   Write-Host "Starting Datadog Agent services"
   & ((Get-ItemProperty "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent").InstallPath + "bin\\agent.exe") restart-service
}
catch [ExitCodeException] {
   Show-Error $_.Exception.Message $_.Exception.LastExitCode
}
catch {
   Show-Error $_.Exception.Message $GENERAL_ERROR_CODE
}
finally {
   Write-Host "Cleaning up..."
   if ($installer -and (Test-Path $installer)) {
      Remove-Item -Force -EA SilentlyContinue $installer
   }
}
Write-Host "Datadog Install Script finished!"
