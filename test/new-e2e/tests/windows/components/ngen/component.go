// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ngen contains code to run ngen.exe precompilation for .NET assemblies in E2E tests
package ngen

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Manager contains the resources to run ngen
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

// New creates a new instance of the ngen component
func New(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("windows-ngen"),
		host:  host,
	}

	if !params.Disabled {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("run"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				RunNgenOnLoadedAssemblies().
				Compile()),
		}, params.ResourceOptions...)
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, cmd)
	}

	return manager, nil
}
