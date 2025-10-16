// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package certificatehost contains code to setup a Windows host for remote certificate testing
package certificatehost

import (
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
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

	// Configure registry for remote access
	cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("set-registry-policy"), &command.Args{
		Create: pulumi.String(`New-Item -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Force -ErrorAction SilentlyContinue; Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -Name LocalAccountTokenFilterPolicy -Value 1 -Type DWORD`),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	// Configure firewall to allow SMB
	cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("configure-firewall"), &command.Args{
		Create: pulumi.String(`New-NetFirewallRule -DisplayName "Allow SMB TCP 445" -Direction Inbound -Protocol TCP -LocalPort 445 -Action Allow -ErrorAction SilentlyContinue`),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	// Start and enable RemoteRegistry service
	cmd, err = host.OS.Runner().Command(manager.namer.ResourceName("start-remote-registry"), &command.Args{
		Create: pulumi.String(powershell.PsHost().
			StartService("RemoteRegistry").
			Compile()),
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
