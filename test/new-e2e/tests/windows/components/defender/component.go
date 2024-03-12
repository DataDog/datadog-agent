// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package defender contains code to control the behavior of Windows defender in the E2E tests
package defender

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	infraComponents "github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Output is an object that models the output of the resource creation
// from the Component.
// See https://www.pulumi.com/docs/concepts/resources/components/#registering-component-outputs
type Output struct {
	infraComponents.JSONImporter
}

// Component is what `run` functions using the defender package will consume
// See https://www.pulumi.com/docs/concepts/resources/components/
type Component struct {
	pulumi.ResourceState
	infraComponents.Component
	namer namer.Namer
	host  *remote.Host
}

// Export registers a key and value pair with the current context's stack.
func (dc *Component) Export(ctx *pulumi.Context, out *Output) error {
	return infraComponents.Export(ctx, dc, out)
}

// NewDefender creates a new instance of the Windows NewDefender component
func NewDefender(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Component, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}
	defenderComp, err := infraComponents.NewComponent(*e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer.WithPrefix(comp.Name())
		comp.host = host
		var err error
		deps := []pulumi.ResourceOption{
			pulumi.Parent(comp),
		}

		if params.Disabled {
			cmd, err := host.OS.Runner().Command(comp.namer.ResourceName("disable-defender"), &command.Args{
				Create: pulumi.String(powershell.PsHost().
					DisableWindowsDefender().
					Compile()),
			}, deps...)
			if err != nil {
				return err
			}
			deps = append(deps, utils.PulumiDependsOn(cmd))
		}

		if params.Uninstall {
			_, err = host.OS.Runner().Command(comp.namer.ResourceName("disable-defender"), &command.Args{
				Create: pulumi.String(powershell.PsHost().
					UninstallWindowsDefender().
					Compile()),
			}, deps...)
		}

		return err
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return defenderComp, nil
}
