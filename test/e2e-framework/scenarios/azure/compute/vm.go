// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure/compute"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewVM creates an Azure VM Instance and returns a Remote component.
// Without any parameter it creates an Windows VM on AMD64 architecture.
func NewVM(e azure.Environment, name string, params ...VMOption) (*remote.Host, error) {
	vmArgs, err := buildArgs(params...)
	if err != nil {
		return nil, err
	}

	// Default missing parameters
	if err = defaultVMArgs(e, vmArgs); err != nil {
		return nil, err
	}

	// Resolve image URN if necessary
	imageInfo, err := resolveOS(e, *vmArgs)
	if err != nil {
		return nil, err
	}

	// Create the Azure VM instance
	return components.NewComponent(&e, e.Namer.ResourceName(name), func(c *remote.Host) error {
		// Create the Azure instance
		c.CloudProvider = pulumi.String(components.CloudProviderAzure).ToStringOutput()
		var err error
		var privateIP pulumi.StringOutput
		var password pulumi.StringOutput

		if vmArgs.osInfo.Family() == os.LinuxFamily {
			_, privateIP, err = compute.NewLinuxInstance(e, c.Name(), imageInfo.urn, vmArgs.instanceType, pulumi.StringPtr(vmArgs.userData), pulumi.Parent(c))
			if err != nil {
				return err
			}
			password = pulumi.String("").ToStringOutput()
		} else if vmArgs.osInfo.Family() == os.WindowsFamily {
			_, privateIP, password, err = compute.NewWindowsInstance(e, c.Name(), imageInfo.urn, vmArgs.instanceType, pulumi.StringPtr(vmArgs.userData), nil, pulumi.Parent(c))
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unsupported OS family %v", vmArgs.osInfo.Family())
		}
		if err != nil {
			return err
		}

		connection, err := remote.NewConnection(
			privateIP,
			compute.AdminUsername,
			remote.WithPrivateKeyPath(e.DefaultPrivateKeyPath()),
			remote.WithPrivateKeyPassword(e.DefaultPrivateKeyPassword()),
		)
		if err != nil {
			return err
		}

		// TODO: Check support of cloud-init on Azure
		return remote.InitHost(&e, connection.ToConnectionOutput(), *vmArgs.osInfo, compute.AdminUsername, password, command.WaitForSuccessfulConnection, c)
	}, vmArgs.pulumiResourceOptions...)
}

func defaultVMArgs(e azure.Environment, vmArgs *vmArgs) error {
	if vmArgs.osInfo == nil {
		vmArgs.osInfo = &os.WindowsServerDefault
	}

	if vmArgs.instanceType == "" {
		vmArgs.instanceType = e.DefaultInstanceType()
		if vmArgs.osInfo.Architecture == os.ARM64Arch {
			vmArgs.instanceType = e.DefaultARMInstanceType()
		}
	}

	return nil
}
