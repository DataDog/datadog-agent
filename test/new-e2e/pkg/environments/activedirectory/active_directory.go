// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package activedirectory

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	infraComponents "github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	pulumiRemote "github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"
)

// Output is an object that models the output of the resource creation
// from the Component.
// See https://www.pulumi.com/docs/concepts/resources/components/#registering-component-outputs
type Output struct {
	infraComponents.JSONImporter
}

// Component is an Active Directory domain component.
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

// NewActiveDirectory creates a new instance of an Active Directory domain deployment
func NewActiveDirectory(ctx *pulumi.Context, e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Component, []pulumi.Resource, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, nil, err
	}

	var createdResources []pulumi.Resource

	domainControllerComp, err := infraComponents.NewComponent(*e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer.WithPrefix(comp.Name())
		comp.host = host

		var installCmd *pulumiRemote.Command
		if params.DomainControllerConfiguration != nil {
			installCmd, err = host.OS.Runner().Command(comp.namer.ResourceName("install-forest"), &command.Args{
				Create: pulumi.String(powershell.PsHost().
					AddActiveDirectoryDomainServicesWindowsFeature().
					ImportActiveDirectoryDomainServicesModule().
					InstallADDSForest(params.DomainControllerConfiguration.DomainName, params.DomainControllerConfiguration.DomainPassword).
					Compile()),
			}, pulumi.Parent(comp))
			if err != nil {
				return err
			}
		}
		createdResources = append(createdResources, installCmd)

		timeProvider, err := time.NewProvider(ctx, comp.namer.ResourceName("time-provider"), &time.ProviderArgs{}, pulumi.DeletedWith(host))
		if err != nil {
			return err
		}
		createdResources = append(createdResources, timeProvider)

		waitForRebootCmd, err := time.NewSleep(ctx, comp.namer.ResourceName("wait-for-host-to-reboot"), &time.SleepArgs{
			CreateDuration: pulumi.String("30s"),
		}, pulumi.Provider(timeProvider), pulumi.DependsOn([]pulumi.Resource{
			installCmd,
		}))
		if err != nil {
			return err
		}
		createdResources = append(createdResources, waitForRebootCmd)

		ensureAdwsStartedCmd, err := host.OS.Runner().Command(comp.namer.ResourceName("ensure-adws-started"), &command.Args{
			Create: pulumi.String(powershell.PsHost().WaitForServiceStatus("ADWS", "Running").Compile()),
		}, pulumi.DependsOn([]pulumi.Resource{
			waitForRebootCmd,
		}))
		if err != nil {
			return err
		}
		createdResources = append(createdResources, ensureAdwsStartedCmd)

		if len(params.DomainUsers) > 0 {
			cmdHost := powershell.PsHost()
			for _, user := range params.DomainUsers {
				cmdHost.AddActiveDirectoryUser(user.Username, user.Password)
			}
			createDomainUsers, err := host.OS.Runner().Command(comp.namer.ResourceName("create-domain-users"), &command.Args{
				Create: pulumi.String(cmdHost.Compile()),
			}, pulumi.DependsOn([]pulumi.Resource{
				ensureAdwsStartedCmd,
			}))
			if err != nil {
				return err
			}
			createdResources = append(createdResources, createDomainUsers)
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, nil, err
	}

	return domainControllerComp, createdResources, nil
}
