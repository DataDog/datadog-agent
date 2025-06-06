// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package driververifier contains code to control the behavior of Driver Verifier in the E2E tests
package driververifier

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Manager contains the resources to manage Driver Verifier
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

type Output struct {
	components.JSONImporter
}

// NewDriverVerifier creates a new instance of the Driver Verifier component
func NewDriverVerifier(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("driver verifier"),
		host:  host,
	}

	var deps []pulumi.ResourceOption
	if !params.Enabled {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("disable driver verifier"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				DisableDriverVerifier().
				Compile()),
		}, deps...)
		if err != nil {
			return nil, err
		}
		deps = append(deps, utils.PulumiDependsOn(cmd))
		manager.Resources = append(manager.Resources, cmd)
	}

	return manager, nil
}
