// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog_installer

import (
	"fmt"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	remoteComp "github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Output is an object that models the output of the resource creation
// from the Component.
// See https://www.pulumi.com/docs/concepts/resources/components/#registering-component-outputs
type Output struct {
	components.JSONImporter
}

// Component is a Datadog Installer component.
// See https://www.pulumi.com/docs/concepts/resources/components/
type Component struct {
	pulumi.ResourceState
	components.Component

	namer namer.Namer
	Host  *remoteComp.Host `pulumi:"host"`
}

func (h *Component) Export(ctx *pulumi.Context, out *Output) error {
	return components.Export(ctx, h, out)
}

// Configuration represents the Windows NewDefender configuration
type Configuration struct {
	Url string
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithInstallUrl specifies the URL to use to retrieve the Datadog Installer
func WithInstallUrl(url string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.Url = url
		return nil
	}
}

// NewConfig creates a default config
func NewConfig(env aws.Environment, options ...Option) (*Configuration, error) {
	if env.PipelineID() != "" {
		options = append([]Option{WithInstallUrl(fmt.Sprintf("https://s3.amazonaws.com/dd-agent-mstesting/pipelines/A7/%s/datadog-installer-1-x86_64.msi", env.PipelineID()))}, options...)
		return common.ApplyOption(&Configuration{}, options)
	} else {
		return nil, fmt.Errorf("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}
}

// NewInstaller creates a new instance of an on-host Agent Installer
func NewInstaller(e aws.Environment, host *remoteComp.Host, options ...Option) (*Component, error) {

	params, err := NewConfig(e, options...)
	if err != nil {
		return nil, err
	}

	hostInstaller, err := components.NewComponent(&e, e.Namer.ResourceName("datadog-installer"), func(comp *Component) error {
		comp.namer = e.CommonNamer().WithPrefix("datadog-installer")
		comp.Host = host

		_, err = host.OS.Runner().Command(comp.namer.ResourceName("install"), &command.Args{
			Create: pulumi.Sprintf(`
Exit (Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /i %s').ExitCode
`, params.Url),
			Delete: pulumi.Sprintf(`
$installerList = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*" | Where-Object {$_.DisplayName -like 'Datadog Installer'}
if (($installerList | measure).Count -ne 1) {
    Write-Error "Could not find the Datadog Installer"
} else {
    cmd /c $installerList.UninstallString
}
`),
		}, pulumi.Parent(comp))
		if err != nil {
			return err
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return hostInstaller, nil
}
