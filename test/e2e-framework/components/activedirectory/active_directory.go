// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package activedirectory

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"
)

// Output is an object that models the output of the resource creation
// from the Component.
// See https://www.pulumi.com/docs/concepts/resources/components/#registering-component-outputs
type Output struct {
	components.JSONImporter
}

// Component is an Active Directory domain component.
// See https://www.pulumi.com/docs/concepts/resources/components/
type Component struct {
	pulumi.ResourceState
	components.Component
	namer namer.Namer
	host  *remote.Host
}

// Export registers a key and value pair with the current context's stack.
func (dc *Component) Export(ctx *pulumi.Context, out *Output) error {
	return components.Export(ctx, dc, out)
}

// A little structure to help manage state for the Active Directory component
type activeDirectoryContext struct {
	pulumiContext    *pulumi.Context
	comp             *Component
	timeProvider     *time.Provider
	createdResources []pulumi.Resource
}

// NewActiveDirectory creates a new instance of an Active Directory domain deployment
// Example usage:
//
//	activeDirectoryComp, activeDirectoryResources, err := activedirectory.NewActiveDirectory(pulumiContext, &awsEnv, host,
//		activedirectory.WithDomainController("datadogqa.lab", "Test1234#"),
//	    activedirectory.WithDomainUser("datadogqa.lab\\ddagentuser", "Test5678#"),
//	)
//	if err != nil {
//		return err
//	}
//	err = activeDirectoryComp.Export(pulumiContext, &env.ActiveDirectory.Output)
//	if err != nil {
//		return err
//	}
func NewActiveDirectory(ctx *pulumi.Context, e config.Env, host *remote.Host, options ...Option) (*Component, []pulumi.Resource, error) {
	if host.OS.Descriptor().Family() != os.WindowsFamily {
		// Print flavor in case OS family don't match, as it's more precise than the family (and the .String() conversion already exists).
		return nil, nil, fmt.Errorf("wrong Operating System family, expected Windows, got %s", host.OS.Descriptor().Flavor.String())
	}

	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, nil, err
	}
	// Tell the component its parent is host and allow to delete it with host. This will prevent uninstalling AD on host delete, we will directly delete the host
	params.ResourceOptions = append([]pulumi.ResourceOption{pulumi.Parent(host), pulumi.DeletedWith(host)}, params.ResourceOptions...)

	adCtx := activeDirectoryContext{
		pulumiContext: ctx,
	}

	domainControllerComp, err := components.NewComponent(e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer().WithPrefix(comp.Name())
		comp.host = host
		adCtx.comp = comp

		// We use the time provider multiple times so instantiate it early
		adCtx.timeProvider, err = time.NewProvider(ctx, comp.namer.ResourceName("time-provider"), &time.ProviderArgs{}, pulumi.DeletedWith(host))
		if err != nil {
			return err
		}
		adCtx.createdResources = append(adCtx.createdResources, adCtx.timeProvider)

		if params.JoinDomainParams != nil {
			err = adCtx.joinActiveDirectoryDomain(params.JoinDomainParams)
			if err != nil {
				return err
			}
		}

		if params.DomainControllerConfiguration != nil {
			err = adCtx.installDomainController(params.DomainControllerConfiguration)
			if err != nil {
				return err
			}
		}

		if len(params.DomainUsers) > 0 {
			// Create users in parallel
			var createUserResources []pulumi.Resource
			var userMaps = make(map[string]struct{})
			for _, user := range params.DomainUsers {
				if _, ok := userMaps[user.Username]; ok {
					return fmt.Errorf("duplicated Active Directory user requested")
				}
				userMaps[user.Username] = struct{}{}

				createDomainUserCmd, err := host.OS.Runner().Command(comp.namer.ResourceName("create-domain-users", user.Username), &command.Args{
					Create: pulumi.Sprintf(`
$HashArguments = @{
	Name = '%s'
	AccountPassword = (ConvertTo-SecureString %s -AsPlainText -Force)
	Enabled = $true
}
$adError = $null
$timeout = [DateTime]::Now.AddSeconds(30)
while ([DateTime]::Now -lt $timeout) {
    try {
        New-ADUser @HashArguments -ErrorAction Stop
        return
    } catch {
        # ERROR_DS_NO_RIDS_ALLOCATED (8208): The directory service was unable to allocate a relative identifier.
        if ($_.FullyQualifiedErrorId.StartsWith('ActiveDirectoryServer:8208')) {
            $adError = $_
            Start-Sleep -Seconds 3
        } else {
            throw
        }
    }
}
throw $adError
`, user.Username, user.Password),
				}, pulumi.DependsOn(adCtx.createdResources))
				if err != nil {
					return err
				}
				createUserResources = append(createUserResources, createDomainUserCmd)
			}
			adCtx.createdResources = append(adCtx.createdResources, createUserResources...)
		}

		return nil
	}, params.ResourceOptions...)
	if err != nil {
		return nil, nil, err
	}

	return domainControllerComp, adCtx.createdResources, nil
}
