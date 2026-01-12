<#
   .SYNOPSIS
   Downloads and installs Datadog on the machine.
#>
[CmdletBinding(DefaultParameterSetName = 'Default')]
$SCRIPT_VERSION = "1.2.1"
$GENERAL_ERROR_CODE = 1

$ddInstallerUrl = $env:DD_INSTALLER_URL
if (-Not $ddInstallerUrl) {
   # Craft the URL to the installer executable
   #
   # Use DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE if it's set,
   # otherwise craft the URL based on the DD_SITE
   #
   # We must not set DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE based on DD_SITE
   # because the environment variable will persist after the script finishes,
   # and a change to DD_SITE won't update the the variable again, which is confusing.
   # The go code at pkg\fleet\installer\oci\download.go will use DD_SITE to determine
   # the registry URL so it's simpler to let it do that.
   if ($env:DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE) {
      $ddInstallerRegistryUrl = $env:DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE
   } else {
      if ($env:DD_SITE -eq "datad0g.com") {
         $ddInstallerRegistryUrl = "install.datad0g.com"
      } else {
         $ddInstallerRegistryUrl = "install.datadoghq.com"
      }
   }
   $ddInstallerUrl = "https://$ddInstallerRegistryUrl/datadog-installer-x86_64.exe"
}

# ExitCodeException can be used to report failures from executables that set $LASTEXITCODE
class ExitCodeException : Exception {
   [string] $LastExitCode

   ExitCodeException($message, $lastExitCode) : base($message) {
      $this.LastExitCode = $lastExitCode
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
      $result = Invoke-WebRequest -Uri $telemetryUrl -Method POST -Body $payload -Headers $requestHeaders -UseBasicParsing
      Write-Host "Sending telemetry: $($result.StatusCode)"
   } catch {
      # Don't propagate errors when sending telemetry, because our error handling code will also
      # try to send telemetry.
      # It's enough to just print a message since there's no further error handling to be done.
      Write-Host "Error sending telemetry"
   }
}

function Test-InstallerIntegrity($installer) {
   if ($env:DD_SKIP_CODE_SIGNING_CHECK) {
      Write-Host "Skipping code signing check"
      return $true
   }
   $signature = Get-AuthenticodeSignature -FilePath $installer

   # We don't expect this value to be localized, it is an enum name
   # https://learn.microsoft.com/en-us/dotnet/api/system.management.automation.signaturestatus
   if ($signature.Status -ne "Valid") {
      throw "Installer signature is not valid: $($signature.StatusMessage)"
   }
   if (-Not ($signature.SignerCertificate.Subject.Contains('CN="Datadog, Inc"'))) {
      throw "Installer is not signed by CN=`"Datadog, Inc`": $($signature.SignerCertificate.Subject)"
   }
   return $true
}

function Show-Error($errorMessage, $errorCode) {
   Write-Host -ForegroundColor Red @"
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
         # Different environments seem to show/hide different output streams
         # PSRemoting and ISE won't see console
         # PSRemoting sees Write-Error but neither console nor ISE do
         # Write-Host seems pretty universal, though we lose the stdout/stderr distinction
         # The only thing we're doing with the output right now is displaying to
         # the user, so this seems okay. If we need the distinction later we can
         # figure out how to output it.
         Write-Host $EventArgs.Data -ForegroundColor 'red'
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

if ($env:SCRIPT_IMPORT_ONLY) {
   # exit if we are just importing the script
   # used so we can test the above functions without running the below installation code
   Exit 0
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
      throw "This script must be run with administrative privileges."
   }

   # Powershell does not enable TLS 1.2 by default, & we want it enabled for faster downloads
   Write-Host "Forcing web requests to TLS v1.2"
   [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor [System.Net.SecurityProtocolType]::Tls12

   $installer = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath "datadog-installer-x86_64.exe"
   if (Test-Path $installer) {
      Remove-Item -Force $installer
   }

   # Check if ddInstallerUrl is a local file path
   if (Test-Path $ddInstallerUrl) {
      Write-Host "Using local installer file: $ddInstallerUrl"
      Copy-Item -Path $ddInstallerUrl -Destination $installer
   } else {
      Write-Host "Downloading installer from $ddInstallerUrl"
      [System.Net.WebClient]::new().DownloadFile($ddInstallerUrl, $installer)
   }

   Write-Host "Verifying installer integrity..."
   if (-Not (Test-InstallerIntegrity $installer)) {
      throw "Installer is not signed by Datadog"
   }
   Write-Host "Installer integrity verified."

   Write-Host "Starting the Datadog installer..."
   $result = Start-ProcessWithOutput -Path $installer
   if ($result -ne 0) {
      # setup only fails if it fails to install to install the Datadog Installer, so it's possible the Agent was not installed
      throw [ExitCodeException]::new("Installer failed", $result)
   }
   Write-Host "Installer completed"

   if (-Not (Test-DatadogAgentPresence)) {
      throw "Agent is not installed"
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
   Exit $_.Exception.LastExitCode
}
catch {
   Show-Error $_.Exception.Message $GENERAL_ERROR_CODE
   Exit $GENERAL_ERROR_CODE
}
finally {
   Write-Host "Cleaning up..."
   if ($installer -and (Test-Path $installer)) {
      Remove-Item -Force -EA SilentlyContinue $installer
   }
}
Write-Host "Datadog Install Script finished!"
