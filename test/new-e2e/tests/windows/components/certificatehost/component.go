// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package certificatehost contains code to setup a Windows host for remote certificate testing
package certificatehost

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

// Manager contains the resources to manage a certificate host setup
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

// NewCertificateHost creates a new instance of the Certificate Host setup component
// This component configures a Windows host to allow remote certificate access via SMB and RemoteRegistry
func NewCertificateHost(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("certificate-host"),
		host:  host,
	}

	var deps []pulumi.ResourceOption

	// Create test user and add to administrators group
	if params.Username != "" && params.Password != "" {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("create-user"), &command.Args{
			Create: pulumi.Sprintf("net user %s %s /add", params.Username, params.Password),
		}, deps...)
		if err != nil {
			return nil, err
		}
		deps = append(deps, utils.PulumiDependsOn(cmd))
		manager.Resources = append(manager.Resources, cmd)

		cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("add-to-admin"), &command.Args{
			Create: pulumi.Sprintf("net localgroup administrators %s /add", params.Username),
		}, deps...)
		if err != nil {
			return nil, err
		}
		deps = append(deps, utils.PulumiDependsOn(cmd))
		manager.Resources = append(manager.Resources, cmd)
	}

	// Configure LocalAccountTokenFilterPolicy to disable UAC remote restrictions for admin accounts
	cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("set-registry-policy"), &command.Args{
		Create: pulumi.String(`New-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Force -ErrorAction SilentlyContinue; Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Name LocalAccountTokenFilterPolicy -Value 1 -Type DWORD`),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	// Enable firewall rules for Remote Registry (uses RPC)
	cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("configure-firewall-remote-registry"), &command.Args{
		Create: pulumi.String(`Enable-NetFirewallRule -DisplayGroup "Remote Service Management" -ErrorAction SilentlyContinue`),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	// Grant the test user full control for remote registry access
	if params.Username != "" {
		cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("grant-registry-permissions"), &command.Args{
			Create: pulumi.Sprintf(`
				$ErrorActionPreference = 'Stop'
				$computerName = $env:COMPUTERNAME
				$identity = "$computerName\%s"
				Write-Output "Granting full registry access for: $identity"
				
				# Grant full control on winreg key (controls remote registry access)
				$winregPath = "HKLM:\SYSTEM\CurrentControlSet\Control\SecurePipeServers\winreg"
				if (-not (Test-Path $winregPath)) {
					New-Item -Path $winregPath -Force | Out-Null
				}
				$winregAcl = Get-Acl $winregPath
				$winregRule = New-Object System.Security.AccessControl.RegistryAccessRule($identity, "FullControl", "None", "None", "Allow")
				$winregAcl.SetAccessRule($winregRule)
				Set-Acl $winregPath $winregAcl
				Write-Output "Set FullControl on winreg"
				
				# Grant full control on certificate store (with inheritance)
				$certPath = "HKLM:\SOFTWARE\Microsoft\SystemCertificates"
				$certAcl = Get-Acl $certPath
				$certRule = New-Object System.Security.AccessControl.RegistryAccessRule($identity, "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow")
				$certAcl.SetAccessRule($certRule)
				Set-Acl $certPath $certAcl
				Write-Output "Set FullControl on certificate store with inheritance"
				Write-Output "SUCCESS: Full registry access configured"
			`, params.Username),
		}, deps...)
		if err != nil {
			return nil, err
		}
		deps = append(deps, utils.PulumiDependsOn(cmd))
		manager.Resources = append(manager.Resources, cmd)
	}

	// Set RemoteRegistry service to automatic startup and start it
	cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("start-remote-registry"), &command.Args{
		Create: pulumi.String(`Set-Service -Name RemoteRegistry -StartupType Automatic; Start-Service -Name RemoteRegistry`),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	// Create self-signed certificate if requested
	if params.CreateSelfSignedCert {
		certSubject := params.CertSubject
		if certSubject == "" {
			certSubject = "CN=test_cert"
		}
		cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("create-certificate"), &command.Args{
			Create: pulumi.Sprintf(`New-SelfSignedCertificate -Subject "%s" -CertStoreLocation "Cert:\LocalMachine\My" -KeyExportPolicy Exportable -KeySpec Signature -KeyLength 2048 -KeyAlgorithm RSA -HashAlgorithm SHA256`, certSubject),
		}, deps...)
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, cmd)
	}

	return manager, nil
}
