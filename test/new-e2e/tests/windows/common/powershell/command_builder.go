// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package powershell provides
package powershell

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

type powerShellCommandBuilder struct {
	cmds []string
}

// PsHost creates a new powerShellCommandBuilder object, which makes it easier to write PowerShell script.
//
//nolint:revive
func PsHost() *powerShellCommandBuilder {
	return &powerShellCommandBuilder{
		// Although host.Execute() will also add `$ErrorActionPreference = "Stop"` to the command,
		// we set it here, too, as PsHost is used in pulumi commands.
		cmds: []string{
			"$ErrorActionPreference = \"Stop\"",
		},
	}
}

// GetLastBootTime uses the win32_operatingsystem Cim class to get the last time the computer was booted.
func (ps *powerShellCommandBuilder) GetLastBootTime() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "(Get-CimInstance -ClassName win32_operatingsystem).lastbootuptime")
	return ps
}

// AddColumn crates a column that appends a column to the previous command output.
func (ps *powerShellCommandBuilder) AddColumn(columnName string, command *powerShellCommandBuilder) *powerShellCommandBuilder {
	ps.cmds[len(ps.cmds)-1] = fmt.Sprintf("%s, @{name='%s'; expression={ %s }}", ps.cmds[len(ps.cmds)-1], columnName, strings.Join(command.cmds, ";"))
	return ps
}

// SelectProperties creates a command that allows selecting some properties from the previous command.
func (ps *powerShellCommandBuilder) SelectProperties(properties ...string) *powerShellCommandBuilder {
	ps.cmds[len(ps.cmds)-1] = fmt.Sprintf("%s | Select-Object -Property %s", ps.cmds[len(ps.cmds)-1], strings.Join(properties, ","))
	return ps
}

// GetPublicIPAddress creates a command that returns the computer's public IP address.
func (ps *powerShellCommandBuilder) GetPublicIPAddress() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "(New-Object System.Net.WebClient).DownloadString('https://ifconfig.me/ip')")
	return ps
}

// ImportActiveDirectoryDomainServicesModule creates a command that imports the PowerShell Active Directory Services module.
func (ps *powerShellCommandBuilder) ImportActiveDirectoryDomainServicesModule() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Import-Module ADDSDeployment")
	return ps
}

// ConvertPasswordToSecureString creates a command that converts a plain text password to a secure string.
func (ps *powerShellCommandBuilder) ConvertPasswordToSecureString(password string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf("ConvertTo-SecureString %s -AsPlainText -Force", password))
	return ps
}

// AddActiveDirectoryDomainServicesWindowsFeature creates a command that installs the Active Directory Domain Services feature.
func (ps *powerShellCommandBuilder) AddActiveDirectoryDomainServicesWindowsFeature() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Add-WindowsFeature -name ad-domain-services -IncludeManagementTools")
	return ps
}

// GetActiveDirectoryDomain creates a command that returns information about the current Active Directory domain.
func (ps *powerShellCommandBuilder) GetActiveDirectoryDomain() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Get-ADDomain")
	return ps
}

// GetActiveDirectoryDomainController creates a command that returns information about the current Active Directory Domain Controller.
func (ps *powerShellCommandBuilder) GetActiveDirectoryDomainController() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Get-ADDomainController")
	return ps
}

// Reboot creates a command that reboots the machine.
func (ps *powerShellCommandBuilder) Reboot() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Restart-Computer -Force")
	return ps
}

// InstallADDSForest creates a command that promotes a server to the role of forest.
func (ps *powerShellCommandBuilder) InstallADDSForest(activeDirectoryDomain, passwd string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
$HashArguments = @{
    CreateDNSDelegation           = $false
    ForestMode                    = "Win2012R2"
    DomainMode                    = "Win2012R2"
    DomainName                    = "%s"
    SafeModeAdministratorPassword = (ConvertTo-SecureString %s -AsPlainText -Force)
    Force                         = $true
}; Install-ADDSForest @HashArguments`, activeDirectoryDomain, passwd))
	return ps
}

// UninstallADDSDomainController creates a command to remove a Domain Controller.
func (ps *powerShellCommandBuilder) UninstallADDSDomainController(passwd string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
$HashArguments = @{
	SkipPreChecks              = $true
    LocalAdministratorPassword = (ConvertTo-SecureString %s -AsPlainText -Force)
    DemoteOperationMasterRole  = $true
	ForceRemoval               = $true
	Force                      = $true
}; Uninstall-ADDSDomainController
`, passwd))
	return ps
}

// AddActiveDirectoryUser creates a command for adding a user to an Active Directory domain.
func (ps *powerShellCommandBuilder) AddActiveDirectoryUser(username, passwd string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
$HashArguments = @{
	Name = '%s'
	AccountPassword = (ConvertTo-SecureString %s -AsPlainText -Force)
	Enabled = $true
}; New-ADUser @HashArguments
`, username, passwd))
	return ps
}

// GetMachineType creates a command that returns the ProductType of the machine (2 for server, 3 for domain controller).
func (ps *powerShellCommandBuilder) GetMachineType() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "(Get-CimInstance -ClassName Win32_OperatingSystem).ProductType")
	return ps
}

// StartService creates a command that starts a given service.
func (ps *powerShellCommandBuilder) StartService(serviceName string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
Start-Service %s
`, serviceName))
	return ps
}

// WaitForServiceStatus creates a command that waits 1 minute for a service to reach a given state.
func (ps *powerShellCommandBuilder) WaitForServiceStatus(serviceName, status string) *powerShellCommandBuilder {
	// TODO: Make timeout configurable
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
(Get-Service %s).WaitForStatus('%s', '00:01:00')
`, serviceName, status))
	return ps
}

// DisableWindowsDefender creates a command to try and disable Windows Defender without uninstalling it
func (ps *powerShellCommandBuilder) DisableWindowsDefender() *powerShellCommandBuilder {
	// ScheduleDay = 8 means never
	ps.cmds = append(ps.cmds, `
if ((Get-MpComputerStatus).IsTamperProtected) {
	Write-Error "Windows Defender is tamper protected, unable to modify settings"
}
(@{DisableArchiveScanning = $true },
 @{DisableRealtimeMonitoring = $true },
 @{DisableBehaviorMonitoring = $true },
 @{MAPSReporting = 0 },
 @{ScanScheduleDay = 8 },
 @{RemediationScheduleDay = 8 }
) | ForEach-Object { Set-MpPreference @_ }`)
	// Even though Microsoft claims to have deprecated this option as of Platform Version 4.18.2108.4,
	// it still works for me on Platform Version 4.18.23110.3 after a reboot, so set it anywawy.
	ps.cmds = append(ps.cmds, `mkdir -Path "HKLM:\SOFTWARE\Policies\Microsoft\Windows Defender"`)
	ps.cmds = append(ps.cmds, `Set-ItemProperty -Path "HKLM:\SOFTWARE\Policies\Microsoft\Windows Defender" -Name DisableAntiSpyware -Value 1`)

	return ps
}

// UninstallWindowsDefender creates a command to uninstall Windows Defender
func (ps *powerShellCommandBuilder) UninstallWindowsDefender() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Uninstall-WindowsFeature -Name Windows-Defender")
	return ps
}

// SetFIPSMode creates a command to enable or disable FIPS mode on the host
func (ps *powerShellCommandBuilder) SetFIPSMode(enabled bool) *powerShellCommandBuilder {
	path := `HKLM:\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy`
	name := "Enabled"
	var value string
	if enabled {
		value = "1"
	} else {
		value = "0"
	}
	typeName := "DWORD"
	cmd := fmt.Sprintf("New-Item -Path '%s' -Force; Set-ItemProperty -Path '%s' -Name '%s' -Value '%s' -Type '%s'", path, path, name, value, typeName)
	ps.cmds = append(ps.cmds, cmd)
	return ps
}

// WaitForMpPreference creates a command that waits for get-mppreference to succeed for 10 minutes
func (ps *powerShellCommandBuilder) WaitForGetMpPreference() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, `
$attempt = 0
$success = $false
$timeout = New-TimeSpan -Seconds 600
$stopwatch = [System.Diagnostics.Stopwatch]::StartNew()

while (-not $success -and $stopwatch.Elapsed -lt $timeout) {
    try {
        $result = Get-MpPreference
        if ($result -ne $null) {
            $success = $true
            Write-Host "Get-MpPreference succeeded on attempt $($attempt + 1)"
        }
    }
    catch {
        Write-Host "Get-MpPreference failed on attempt $($attempt + 1): $($_.Exception.Message)"
    }
    
    if (-not $success) {
        $attempt++
        Start-Sleep -Seconds 2
    }
}

$stopwatch.Stop()

if (-not $success -and $stopwatch.Elapsed -ge $timeout) {
    Write-Error "Get-MpPreference failed after $($stopwatch.Elapsed.TotalSeconds) seconds and $attempt attempts: $($Error[0].Exception.Message)"
}`)
	return ps
}

// Execute compiles the list of PowerShell commands into one script and runs it on the given host
func (ps *powerShellCommandBuilder) Execute(host *components.RemoteHost) (string, error) {
	return host.Execute(ps.Compile())
}

// Compile joins all the saved command into one valid PowerShell script command.
func (ps *powerShellCommandBuilder) Compile() string {
	return strings.Join(ps.cmds, ";")
}

// EnableTestSigning creates a command that enables TestSigning
func (ps *powerShellCommandBuilder) EnableTestSigning() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, `
$result = bcdedit.exe | findstr "testsigning" | findstr "Yes"
if ($result -eq $null) {
	bcdedit.exe /set testsigning on
	Restart-Computer -Force
}
else {
	Write-Host "TestSigning is already enabled"
}
`)
	return ps
}
