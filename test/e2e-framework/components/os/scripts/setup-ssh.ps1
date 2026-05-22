# Start a transcript so the full output of this script lands at C:\e2e-setup.log on the VM. 
try {
  $null = Start-Transcript -Path 'C:\e2e-setup.log' -Append -ErrorAction SilentlyContinue
} catch {
  # If transcript can't start (e.g., already running), proceed without it.
}

# Wrap the rest of the script in try/finally so the transcript is always closed,
# even if a 'throw' inside this script aborts execution early.
try {

# function to test if the OS is Windows Server 2025
function Is-WindowsServer2025 {
  $osInfo = Get-CimInstance -ClassName Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber
  $isWindowsServer2025 = $osInfo.Caption -like "*Windows Server 2025*"
  return $isWindowsServer2025
}

# function to test if the sshd service is running and if it needs to be replaced
function Test-SshInstallationNeeded {
  $service = Get-Service -Name sshd -ErrorAction SilentlyContinue

  if ($service -ne $null) {
    Write-Host "Stop sshd service"
    Stop-Service sshd
    if (Is-WindowsServer2025) {
      # for Windows Server 2025, replace the service
      return $true
    }
  } else {
    return $true
  }
  return $false
}

# function to restore the auto inherited flag without affecting the DACL
# This function applies to Windows Server 2025 only
# WINA-1694: The root drive on Windows Server 2025 is missing the SE_DACL_AUTO_INHERITED flag
# This causes new files/directories under the root drive to have incorrect permissions,
# which causes failures in some of our E2E tests.
# Microsoft support case ID: 2508040010002067
# https://serviceshub.microsoft.com/support/case/2508040010002067?workspaceId=687f2284-0ce3-40c9-8d3e-ebf747c76eab
function Restore-AutoInheritedFlag {
   if (-not (Is-WindowsServer2025)) {
    return
  } else {
    # first add the following command: icacls.exe C:\ /inheritance:d
    # this disables inheritance on the C:\ drive
    icacls.exe C:\ /inheritance:d
  }
}

# function to create a universal SSH firewall rule
# Windows Server 2025 runs preinstalled SSH but the rule is specific to a different binary path.
# The MSI creates a rule as well but it only applies to private profiles. Here we create our
# own rule that's more permissive for the testing environment.
function Set-SshFirewallConfiguration {
  # return if the OS is not Windows server 2025
  if (-not (Is-WindowsServer2025)) {
    return
  }

  Write-Host "Creating universal SSH firewall rule..."
  try {
    $ruleName = "SSH-Server-DD-Universal"
    if (-not (Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue)) {
      New-NetFirewallRule -Name $ruleName `
              -DisplayName 'SSH Server (Universal)' `
              -Description 'Allow SSH inbound connections on port 22' `
              -Enabled True `
              -Direction Inbound `
              -Protocol TCP `
              -LocalPort 22 `
              -Action Allow `
              -Profile Any `
              -RemoteAddress Any `
              -EdgeTraversalPolicy Allow
                
      Write-Host "Universal SSH firewall rule created"
    } else {
        Write-Host "Universal SSH firewall rule already exists"
    }
  } catch {
    Write-Warning "Failed to create SSH firewall rule: $($_.Exception.Message)"
  }
}

# Install PowerShell 7 and switch OpenSSH's default shell to it. The stock AWS Windows
# AMI's `powershell.exe` (5.1) pays a multi-second JIT tax on every cold start because
# its assemblies are not pre-NGen'd, which under load pins CPU and can cause flaky
# issues from resource contention. PowerShell 7 (pwsh.exe) is AOT-compiled and starts
# in ~500 ms cold with a tight spread, so we side-step the issue entirely instead of
# trying to warm the .NET Framework native image cache.
#
# We still disable the .NET Framework NGEN scheduled tasks so the OS-driven NGen
# update doesn't kick in mid-test and steal CPU/IO at an inopportune moment.
# Ref: ACIX-1057
$pwshUrl = 'https://github.com/PowerShell/PowerShell/releases/download/v7.6.1/PowerShell-7.6.1-win-x64.msi'
$pwshPath = 'C:\Program Files\PowerShell\7\pwsh.exe'

function Disable-NgenTasks {
  # Disable all .NET Framework NGEN scheduled tasks. Discover by pattern rather than
  # hardcoding names — task names include the .NET Framework version and that changes
  # between Windows releases. Skip already-disabled tasks (e.g. *Critical) since
  # Disable-ScheduledTask on an already-disabled task succeeds but the no-op log line
  # is noise.
  $ngenTasks = Get-ScheduledTask -TaskPath '\Microsoft\Windows\.NET Framework\' -ErrorAction SilentlyContinue |
               Where-Object { $_.TaskName -like '*NGEN*' -and $_.State -ne 'Disabled' }
  foreach ($task in $ngenTasks) {
    try {
      Disable-ScheduledTask -TaskPath '\Microsoft\Windows\.NET Framework\' -TaskName $task.TaskName -ErrorAction Stop | Out-Null
      Write-Host "Disabled scheduled task: $($task.TaskName)"
    } catch {
      Write-Warning "Failed to disable scheduled task '$($task.TaskName)': $($_.Exception.Message)"
    }
  }
}

function Install-PowerShell7 {
  param([string]$Url, [string]$ExpectedPath)
  if (Test-Path $ExpectedPath) {
    Write-Host "PowerShell 7 already installed at $ExpectedPath"
    return
  }
  # MSI properties:
  #   USE_MU=0 / ENABLE_MU=0          : do not enrol pwsh into Microsoft Update
  #   DISABLE_TELEMETRY=1             : set POWERSHELL_TELEMETRY_OPTOUT for all users
  #   ADD_PATH=1                      : prepend the install dir to system PATH
  #   ENABLE_PSREMOTING=0             : don't register the WinRM endpoint for pwsh
  #   ADD_FILE_CONTEXT_MENU_RUNPOWERSHELL=1
  #   ADD_EXPLORER_CONTEXT_MENU_OPENPOWERSHELL=1
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $msiArgs = @(
    '/i', $Url, '/quiet', '/norestart',
    'USE_MU=0', 'ENABLE_MU=0', 'DISABLE_TELEMETRY=1', 'ADD_PATH=1',
    'ENABLE_PSREMOTING=0',
    'ADD_FILE_CONTEXT_MENU_RUNPOWERSHELL=1',
    'ADD_EXPLORER_CONTEXT_MENU_OPENPOWERSHELL=1'
  )
  $proc = Start-Process -FilePath 'msiexec.exe' -ArgumentList $msiArgs -Wait -PassThru
  $sw.Stop()
  if ($proc.ExitCode -ne 0) {
    throw "PowerShell 7 MSI install failed: exit code $($proc.ExitCode)"
  }
  Write-Host ("PowerShell 7 MSI installed in {0:N1} s" -f $sw.Elapsed.TotalSeconds)

  if (-not (Test-Path $ExpectedPath)) {
    throw "PowerShell 7 install succeeded but $ExpectedPath is missing"
  }

  # pwsh defaults to ANSI-colored output even over SSH, which corrupts test
  # framework parsers that expect plain text — output gets prefixed with ESC
  # escape sequences. Set NO_COLOR=1 at Machine scope so new SSH sessions inherit
  # it and pwsh switches $PSStyle.OutputRendering to PlainText.
  # https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_ansi_terminals?view=powershell-7.4#disabling-ansi-output
  [Environment]::SetEnvironmentVariable('NO_COLOR', '1', 'Machine')
  Write-Host "Set NO_COLOR=1 (Machine) so pwsh emits plain text over SSH"
}

# Main script execution

# Install pwsh BEFORE OpenSSH so the OpenSSH MSI's DefaultShell registration below can
# point straight at pwsh.exe and we never have to flip it from powershell.exe later.
Disable-NgenTasks
Install-PowerShell7 -Url $pwshUrl -ExpectedPath $pwshPath

if (Test-SshInstallationNeeded) {
  Write-Host "sshd service not found or needs replacement, installing OpenSSH Server"
  # Add-WindowsCapability does NOT install a consistent version across Windows versions, this lead to
  # compatibility issues (different command line quoting rules).
  # Prefer installing sshd via MSI  
  $res = start-process -passthru -wait msiexec.exe -args '/i https://github.com/PowerShell/Win32-OpenSSH/releases/download/v9.5.0.0p1-Beta/OpenSSH-Win64-v9.5.0.0.msi /qn'
  if ($res.ExitCode -ne 0) {
    throw "SSH install failed: $($res.ExitCode)"
  }
  Write-Host "OpenSSH Server installed"
  $retries = 0
  # Confirm the Firewall rule is configured. It should be created automatically by setup. Run the following to verify
  while (!(Get-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -ErrorAction SilentlyContinue).Enabled) {
    if ($retries -ge 10) {
      throw "Firewall rule 'OpenSSH-Server-In-TCP' not found after 10 retries"
    }
    if ($retries -gt 0) {
      Start-Sleep -Seconds 5
    }
    Write-Output "Firewall Rule 'OpenSSH-Server-In-TCP' does not exist, creating it..."
    New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
    $retries++
  } 
  Write-Output "Firewall rule 'OpenSSH-Server-In-TCP' created."
  # Point OpenSSH at pwsh.exe (installed earlier) instead of the stock powershell.exe.
  $retries = 0
  $res = Get-ItemProperty "HKLM:\SOFTWARE\OpenSSH"
  while ((Get-ItemProperty "HKLM:\SOFTWARE\OpenSSH").DefaultShell -ne $pwshPath) {
    if ($retries -ge 10) {
      throw "Failed to set powershell as default shell for sshd after 10 retries"
    }
    if ($retries -gt 0) {
      Start-Sleep -Seconds 5
    }
    Write-Host "Set powershell as default shell for sshd"
    New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell -Value $pwshPath -PropertyType String -Force 
    $retries++
  }
  $retries = 0
  while (((Get-Service -Name sshd -ErrorAction SilentlyContinue) -eq $null) -and ($waitLeft -gt 0)) {
    if ($retries -ge 10) {
      throw "Failed to find sshd service after 10 retries, 5 seconds interval"
    }
    Write-Host "Waiting for sshd service to exist"
    Start-Sleep -Seconds 5
    $retries++
  }
  $retries = 0
  while ((Get-Service -Name sshd -ErrorAction SilentlyContinue).StartType -ne "Automatic") {
    if ($retries -ge 10) {
      throw "Failed to set sshd service to start automatically after 10 retries"
    }
    if ($retries -gt 0) {
      Start-Sleep -Seconds 5
    }
    Write-Host "Set sshd service to start automatically"
    Set-Service -Name sshd -StartupType Automatic
    $retries++
  }
}

Restore-AutoInheritedFlag
Set-SshFirewallConfiguration

# Disable Edge auto-updates to avoid interference during E2E tests (high resource usage)
Write-Host "Disabling Edge auto-updates..."
try {
  Rename-Item -Path "C:\Program Files (x86)\Microsoft\EdgeUpdate\MicrosoftEdgeUpdate.exe" -NewName "Disabled_MicrosoftEdgeUpdate.exe" -ErrorAction Stop
  Write-Host "Edge auto-updates disabled"
} catch {
  Write-Warning "Failed to disable Edge auto-updates: $($_.Exception.Message)"
}

Write-Host "Resetting ssh authorized keys"
$retries = 0
while (Test-Path $env:ProgramData\ssh\administrators_authorized_keys) { 
  if ($retries -ge 10) {
    throw "Failed to remove existing administrators_authorized_keys file after 10 retries"
  }
  if ($retries -gt 0) {
    Start-Sleep -Seconds 1
  }
  Write-Host "Remove existing administrators_authorized_keys file"
  Remove-Item $env:ProgramData\ssh\administrators_authorized_keys
  $retries++
}

$retries = 0
while (-not (Test-Path $env:ProgramData\ssh\administrators_authorized_keys)) { 
  if ($retries -ge 10) {
    throw "Failed to create administrators_authorized_keys file after 10 retries"
  }
  if ($retries -gt 0) {
    Start-Sleep -Seconds 1
  }
  Write-Host "Creating administrators_authorized_keys file"
  New-Item -Path $env:ProgramData\ssh -Name administrators_authorized_keys -ItemType file
  $retries++
}
Add-Content -Path $env:ProgramData\ssh\administrators_authorized_keys -Value $authorizedKey
icacls.exe ""$env:ProgramData\ssh\administrators_authorized_keys"" /inheritance:r /grant ""Administrators:F"" /grant ""SYSTEM:F""
# Start sshd service
$retries = 0
while ((Get-Service -Name sshd -ErrorAction SilentlyContinue).Status -ne "Running") {
  if ($retries -ge 10) {
    throw "Failed to start sshd service after 10 retries"
  }
  if ($retries -gt 0) {
    Start-Sleep -Seconds 5
  }
  Write-Host "Starting sshd service"
  Start-Service sshd
  $retries++
}

} finally {
  # Always close the transcript so the log file is fully flushed even if an earlier
  # throw aborted the script.
  try { Stop-Transcript -ErrorAction SilentlyContinue | Out-Null } catch { }
}

