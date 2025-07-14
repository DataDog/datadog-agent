// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package defender contains code to control the behavior of Windows defender in the E2E tests
package defender

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

// Manager contains the resources to manage Windows Defender
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

// NewDefender creates a new instance of the Windows NewDefender component
func NewDefender(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("windows-defender"),
		host:  host,
	}

	var deps []pulumi.ResourceOption
	cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("get-defender-status"), &command.Args{
		Create: pulumi.String(powershell.PsHost().
			WaitForServiceStatus("WinDefend", "Running").
			Compile()),
	}, deps...)
	if err != nil {
		return nil, err
	}
	deps = append(deps, utils.PulumiDependsOn(cmd))
	manager.Resources = append(manager.Resources, cmd)

	if params.Disabled {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("disable-defender"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				DisableWindowsDefender().
				Compile()),
		}, deps...)
		if err != nil {
			return nil, err
		}
		deps = append(deps, utils.PulumiDependsOn(cmd))
		manager.Resources = append(manager.Resources, cmd)
	}

	if params.Uninstall {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("uninstall-defender"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				UninstallWindowsDefender().
				Compile()),
		}, deps...)
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, cmd)
	}

	return manager, nil
}
