// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"strings"
)

type powerShellCommandBuilder struct {
	cmds []string
}

// PsHost creates a new powerShellCommandBuilder object
func PsHost() *powerShellCommandBuilder {
	return &powerShellCommandBuilder{}
}

// GetLastBootTime uses the win32_operatingsystem Cim class to get the last time the computer was booted
func (ps *powerShellCommandBuilder) GetLastBootTime() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "(Get-CimInstance -ClassName win32_operatingsystem).lastbootuptime")
	return ps
}

// AddColumn appends a column to the previous command output
func (ps *powerShellCommandBuilder) AddColumn(columnName string, command *powerShellCommandBuilder) *powerShellCommandBuilder {
	ps.cmds[len(ps.cmds)-1] = fmt.Sprintf("%s, @{name='%s'; expression={ %s }}", ps.cmds[len(ps.cmds)-1], columnName, strings.Join(command.cmds, ";"))
	return ps
}

func (ps *powerShellCommandBuilder) SelectProperties(properties ...string) *powerShellCommandBuilder {
	ps.cmds[len(ps.cmds)-1] = fmt.Sprintf("%s | Select-Object -Property %s", ps.cmds[len(ps.cmds)-1], strings.Join(properties, ","))
	return ps
}

// GetPublicIPAddress returns the computer's public IP address
func (ps *powerShellCommandBuilder) GetPublicIPAddress() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "(New-Object System.Net.WebClient).DownloadString('https://ifconfig.me/ip')")
	return ps
}

func (ps *powerShellCommandBuilder) ImportActiveDirectoryDomainServicesModule() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Import-Module ADDSDeployment")
	return ps
}

func (ps *powerShellCommandBuilder) ConvertPasswordToSecureString(password string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf("ConvertTo-SecureString %s -AsPlainText -Force", password))
	return ps
}

func (ps *powerShellCommandBuilder) AddActiveDirectoryDomainServicesWindowsFeature() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Add-WindowsFeature -name ad-domain-services -IncludeManagementTools")
	return ps
}

func (ps *powerShellCommandBuilder) GetActiveDirectoryDomain() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Get-ADDomain")
	return ps
}

func (ps *powerShellCommandBuilder) GetActiveDirectoryDomainController() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Get-ADDomainController")
	return ps
}

func (ps *powerShellCommandBuilder) Reboot() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, "Restart-Computer -Force")
	return ps
}

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

func (ps *powerShellCommandBuilder) GetMachineType() *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
(Get-CimInstance -ClassName Win32_OperatingSystem).ProductType
`))
	return ps
}

func (ps *powerShellCommandBuilder) StartService(serviceName string) *powerShellCommandBuilder {
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
Start-Service %s
`, serviceName))
	return ps
}

func (ps *powerShellCommandBuilder) WaitForServiceStatus(serviceName, status string) *powerShellCommandBuilder {
	// TODO: Make timeout configurable
	ps.cmds = append(ps.cmds, fmt.Sprintf(`
(Get-Service %s).WaitForStatus('%s', '00:01:00')
`, serviceName, status))
	return ps
}

// Execute compiles the list of PowerShell commands into one script and runs it on the given host
func (ps *powerShellCommandBuilder) Execute(host *components.RemoteHost) (string, error) {
	return host.Execute(ps.Compile())
}

func (ps *powerShellCommandBuilder) Compile() string {
	return strings.Join(ps.cmds, ";")
}
