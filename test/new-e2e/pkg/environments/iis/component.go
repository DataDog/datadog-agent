// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package iis contains code for the IIS component
package iis

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	infraComponents "github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"
)

// Output is an object that models the output of the resource creation
// from the Component.
// See https://www.pulumi.com/docs/concepts/resources/components/#registering-component-outputs
type Output struct {
	infraComponents.JSONImporter
}

// Component represent an IIServer and its sites.
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

// NewServer creates a new IIS server component.
func NewServer(ctx *pulumi.Context, e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Component, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	iisComponent, err := infraComponents.NewComponent(*e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer.WithPrefix(comp.Name())
		comp.host = host

		installIISCommand, err := host.OS.Runner().Command(comp.namer.ResourceName("install-iis"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				InstallIIS().
				Compile()),
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
			Create: pulumi.String(powershell.PsHost().WaitForServiceStatus("W3SVC", "Running").Compile()),
		}, pulumi.DependsOn([]pulumi.Resource{
			waitForRebootCmd,
		}))
		if err != nil {
			return err
		}

		if len(params.Sites) > 0 {
			for _, site := range params.Sites {
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
					Create: pulumi.String(powershell.PsHost().
						NewIISSite(site.Name, site.BindingPort, site.TargetAssetsDir).
						Compile()),
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
