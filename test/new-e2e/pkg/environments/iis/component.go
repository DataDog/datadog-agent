// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package iis

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
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

// Component is an Active Directory domain component.
// See https://www.pulumi.com/docs/concepts/resources/components/
type Component struct {
	pulumi.ResourceState
	infraComponents.Component
	namer namer.Namer
	host  *remote.Host
}

var (
	//go:embed scripts/installiis.ps1
	installIISScript []byte
)

// Export registers a key and value pair with the current context's stack.
func (dc *Component) Export(ctx *pulumi.Context, out *Output) error {
	return infraComponents.Export(ctx, dc, out)
}

// NewIISServer creates a new instance of an IIS deployment
func NewIISServer(ctx *pulumi.Context, e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Component, error) {
	params, err := common.ApplyOption(&Configuration{
		// JL: Should we set sensible defaults here ?
	}, options)
	if err != nil {
		return nil, err
	}

	domainControllerComp, err := infraComponents.NewComponent(*e, host.Name(), func(comp *Component) error {
		comp.namer = e.CommonNamer.WithPrefix(comp.Name())
		comp.host = host

		scriptFile := `c:\temp\install-iis.ps1`
		_, err := host.OS.FileManager().CreateDirectory("C:\\temp", false)
		if err != nil {
			return err
		}
		_, err = host.OS.FileManager().CopyInlineFile(pulumi.String(installIISScript), "C:\\temp", false)
		if err != nil {
			return err
		}

		excmd, err := host.OS.Runner().Command(comp.namer.ResourceName("install-iis"), &command.Args{
			Create: pulumi.String(fmt.Sprintf("powershell -ExecutionPolicy Bypass -File %s", scriptFile)),
		}, pulumi.Parent(comp))
		if err != nil {
			return fmt.Errorf("failed to install IIS: %w", err)
		}

		for _, s := range params.Sites {

			// create the site directory
			//tgtpath := fmt.Sprintf("c:\\tmp\\inetpub\\%s", s.Name)
			tgtpath := filepath.Join("c:", "tmp", "inetpub", s.Name)
			_, err := host.OS.FileManager().CreateDirectory(tgtpath, false)
			if err != nil {
				return err
			}

			if s.AssetsDir != "" {
				// copy the assets
				host.OS.FileManager().CopyAbsoluteFolder(s.AssetsDir, tgtpath)
				//host.CopyFolder(s.AssetsDir, tgtpath)
			}
			script := `
			if ((get-iissite -name %s).State -ne "Started") {
				New-IISSite -ErrorAction SilentlyContinue -Name %s -BindingInformation '%s' -PhysicalPath %s
			}`
			wintgtpath := strings.Replace(tgtpath, "/", "\\", -1)
			cmd := fmt.Sprintf(script, s.Name, s.Name, s.BindingPort, wintgtpath)

			_, err = host.OS.Runner().Command(comp.namer.ResourceName("create-iis-site"), &command.Args{
				Create: pulumi.String(cmd),
			}, pulumi.DependsOn([]pulumi.Resource{
				excmd,
			}))
			//output, err := host.Execute(cmd)
			if err != nil {
				return fmt.Errorf("failed to create IIS site: %w\n", err)
			}
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return domainControllerComp, nil
}
