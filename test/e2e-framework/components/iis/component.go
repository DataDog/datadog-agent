// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package iis contains code for the IIS component
package iis

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
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

// Component represent an IIServer and its sites.
// See https://www.pulumi.com/docs/concepts/resources/components/
type Component struct {
	pulumi.ResourceState
	components.Component
	namer namer.Namer
	host  *remote.Host
}

// Export registers a key and value pair with the current context's stack.
func (c *Component) Export(ctx *pulumi.Context, out *Output) error {
	return components.Export(ctx, c, out)
}

// NewServer creates a new IIS server component.
// Example usage:
//
//	iisServer, err := iis.NewServer(ctx, awsEnv.CommonEnvironment, vm,
//		iis.WithSite(iis.SiteDefinition{
//			Name:            "TestSite1",
//			BindingPort:     "*:8081:",
//			SourceAssetsDir: srcDir,
//		}),
//	)
//	if err != nil {
//		return err
//	}
//	err = iisServer.Export(ctx, env.IISServer)
func NewServer(ctx *pulumi.Context, e config.Env, host *remote.Host, options ...Option) (*Component, error) {
	if host.OS.Descriptor().Family() != os.WindowsFamily {
		// Print flavor in case OS family don't match, as it's more precise than the family (and the .String() conversion already exists).
		return nil, fmt.Errorf("wrong Operating System family, expected Windows, got %s", host.OS.Descriptor().Flavor.String())
	}

	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	iisComponent, err := components.NewComponent(e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer().WithPrefix(comp.Name())
		comp.host = host

		installIISCommand, err := host.OS.Runner().Command(comp.namer.ResourceName("install-iis"), &command.Args{
			Create: pulumi.String(`
function ExitWithCode($exitcode) {
	$host.SetShouldExit($exitcode)
	exit $exitcode
}
$result = Install-Windowsfeature -name Web-Server -IncludeManagementTools
if (! $result.Success ) {
	exit -1
}
if ($result.RestartNeeded -eq "Yes") {
	ExitWithCode(3010)
}
`),
		}, pulumi.Parent(comp))
		if err != nil {
			return err
		}

		timeProvider, err := time.NewProvider(ctx, comp.namer.ResourceName("time-provider"), &time.ProviderArgs{}, pulumi.DeletedWith(host))
		if err != nil {
			return err
		}

		waitForRebootCmd, err := time.NewSleep(ctx, comp.namer.ResourceName("wait-for-host-to-reboot"), &time.SleepArgs{
			CreateDuration: pulumi.String("30s"),
		}, pulumi.Provider(timeProvider), pulumi.DependsOn([]pulumi.Resource{
			installIISCommand,
		}))
		if err != nil {
			return err
		}

		ensureIISStarted, err := host.OS.Runner().Command(comp.namer.ResourceName("ensure-iis-started"), &command.Args{
			Create: pulumi.String(`(Get-Service W3SVC).WaitForStatus('Running', '00:01:00')`),
		}, pulumi.DependsOn([]pulumi.Resource{
			waitForRebootCmd,
		}))
		if err != nil {
			return err
		}

		if len(params.Sites) > 0 {
			var sitesMap = make(map[string]struct{})
			for _, site := range params.Sites {
				if _, ok := sitesMap[site.Name]; ok {
					return fmt.Errorf("duplicated IIS Site requested")
				}
				sitesMap[site.Name] = struct{}{}

				dependencies := []pulumi.ResourceOption{
					utils.PulumiDependsOn(ensureIISStarted),
				}

				if site.TargetAssetsDir == "" {
					site.TargetAssetsDir = fmt.Sprintf("C:\\inetpub\\%s", site.Name)
				}

				if site.SourceAssetsDir != "" {
					copyAssets, err := host.OS.FileManager().CopyAbsoluteFolder(site.SourceAssetsDir, site.TargetAssetsDir, dependencies...)
					if err != nil {
						return err
					}
					dependencies = append(dependencies, pulumi.DependsOn(copyAssets))
				}

				_, err = host.OS.Runner().Command(comp.namer.ResourceName("create-iis-site", site.Name), &command.Args{
					Create: pulumi.Sprintf(`New-IISSite -Name '%s' -BindingInformation '%s' -PhysicalPath '%s'`, site.Name, site.BindingPort, site.TargetAssetsDir),
				}, dependencies...)
				if err != nil {
					return err
				}
			}

			if err != nil {
				return err
			}
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return iisComponent, nil
}
