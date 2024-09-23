// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer defines a Pulumi component for installing the Datadog Installer on a remote host in the
// provisioning step.
package installer

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	remoteComp "github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
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

// Export exports the output of this component
func (h *Component) Export(ctx *pulumi.Context, out *Output) error {
	return components.Export(ctx, h, out)
}

// Configuration represents the Windows NewDefender configuration
type Configuration struct {
	URL string
}

// Option is an optional function parameter type for Configuration options
type Option = func(*Configuration) error

// WithInstallURL specifies the URL to use to retrieve the Datadog Installer
func WithInstallURL(url string) func(*Configuration) error {
	return func(p *Configuration) error {
		p.URL = url
		return nil
	}
}

// NewConfig creates a default config
func NewConfig(env config.Env, options ...Option) (*Configuration, error) {
	if env.PipelineID() != "" {
		artifactURL, err := pipeline.GetPipelineArtifact(env.PipelineID(), pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".msi")
		})
		if err != nil {
			return nil, err
		}
		options = append([]Option{WithInstallURL(artifactURL)}, options...)
	}
	return common.ApplyOption(&Configuration{}, options)
}

// NewInstaller creates a new instance of an on-host Agent Installer
func NewInstaller(e config.Env, host *remoteComp.Host, options ...Option) (*Component, error) {

	params, err := NewConfig(e, options...)
	if err != nil {
		return nil, err
	}

	hostInstaller, err := components.NewComponent(e, e.CommonNamer().ResourceName("datadog-installer"), func(comp *Component) error {
		comp.namer = e.CommonNamer().WithPrefix("datadog-installer")
		comp.Host = host

		_, err = host.OS.Runner().Command(comp.namer.ResourceName("install"), &command.Args{
			Create: pulumi.Sprintf(`
Exit (Start-Process -Wait msiexec -PassThru -ArgumentList '/qn /i %s').ExitCode
`, params.URL),
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
