// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package installer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type installerLabVMArgs struct {
	name              string
	descriptor        os.Descriptor
	instanceType      string
	extraPackageNames []string
}

var installerLabVMs = []installerLabVMArgs{
	{
		name:              "ubuntu-22",
		descriptor:        os.NewDescriptorWithArch(os.Ubuntu, "22.04", os.ARM64Arch),
		instanceType:      "t4g.medium",
		extraPackageNames: []string{},
	},
	{
		name:              "ubuntu-20",
		descriptor:        os.NewDescriptorWithArch(os.Ubuntu, "20.04", os.ARM64Arch),
		instanceType:      "t4g.medium",
		extraPackageNames: []string{},
	},
	{
		name:              "debian-12",
		descriptor:        os.NewDescriptorWithArch(os.Debian, "12", os.ARM64Arch),
		instanceType:      "t4g.medium",
		extraPackageNames: []string{},
	},
	{
		name:              "debian-12-small",
		descriptor:        os.NewDescriptorWithArch(os.Debian, "12", os.ARM64Arch),
		instanceType:      "t4g.small",
		extraPackageNames: []string{},
	},
	{
		name:              "suse-15",
		descriptor:        os.NewDescriptorWithArch(os.Suse, "15-sp4", os.ARM64Arch),
		instanceType:      "t4g.medium",
		extraPackageNames: []string{},
	},
}

const installScriptFormat = `#!/bin/bash
DD_API_KEY=%s DD_HOSTNAME=%s DD_SITE=%s DD_REMOTE_UPDATES=true bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"
`

const hostnamePrefix = "installer-lab-%s"

type LabHost struct {
	pulumi.ResourceState
	components.Component

	namer namer.Namer
	host  *remoteComp.Host
}

func Run(ctx *pulumi.Context) error {
	env, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	for _, vmArgs := range installerLabVMs {
		vm, err := ec2.NewVM(
			env,
			vmArgs.name,
			ec2.WithInstanceType(vmArgs.instanceType),
			ec2.WithOSArch(vmArgs.descriptor, vmArgs.descriptor.Architecture),
		)
		if err != nil {
			return err
		}
		if err := vm.Export(ctx, nil); err != nil {
			return err
		}

		_, err = components.NewComponent(&env, vm.Name(), func(comp *LabHost) error {
			comp.namer = env.CommonNamer().WithPrefix(comp.Name())
			comp.host = vm

			err := comp.installManagedAgent(env.AgentAPIKey(), fmt.Sprintf(hostnamePrefix, vmArgs.name), env.Site())

			return err
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *LabHost) installManagedAgent(
	apiKey pulumi.StringOutput, hostname string, site string,
) error {
	installScript := pulumi.Sprintf(installScriptFormat, apiKey, hostname, site)

	_, err := h.host.OS.Runner().Command(
		h.namer.ResourceName("install-script"),
		&command.Args{
			Create: installScript,
		},
	)

	return err
}
