// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewLinuxInstance(e gcp.Environment, name string, imageName string, instanceType string, nestedVirt bool, opts ...pulumi.ResourceOption) (*compute.Instance, error) {

	sshPublicKey, err := utils.GetSSHPublicKey(e.DefaultPublicKeyPath())
	if err != nil {
		return nil, err
	}
	instance, err := compute.NewInstance(e.Ctx(), e.Namer.ResourceName(name), &compute.InstanceArgs{
		NetworkInterfaces: compute.InstanceNetworkInterfaceArray{
			&compute.InstanceNetworkInterfaceArgs{
				AccessConfigs: compute.InstanceNetworkInterfaceAccessConfigArray{
					&compute.InstanceNetworkInterfaceAccessConfigArgs{
						NatIp: pulumi.String(""),
					},
				},
				Network:    pulumi.String(e.DefaultNetworkName()),
				Subnetwork: pulumi.String(e.DefaultSubnet()),
			},
		},
		Name:        e.Namer.DisplayName(63, pulumi.String(name)),
		MachineType: pulumi.String(instanceType),
		AdvancedMachineFeatures: &compute.InstanceAdvancedMachineFeaturesArgs{
			EnableNestedVirtualization: pulumi.BoolPtr(nestedVirt),
		},
		Tags: pulumi.StringArray{
			pulumi.String("appgate-gateway"),
			pulumi.String("nat-us-central1"),
		},
		BootDisk: &compute.InstanceBootDiskArgs{
			InitializeParams: &compute.InstanceBootDiskInitializeParamsArgs{
				Image: pulumi.String(imageName),
				Labels: pulumi.StringMap{
					"my_label": pulumi.String("value"),
				},
				Size: pulumi.Int(100),
			},
		},
		Metadata: pulumi.StringMap{
			"enable-oslogin": pulumi.String("false"),
			"ssh-keys":       pulumi.Sprintf("gce:%s", sshPublicKey),
		},
		ServiceAccount: &compute.InstanceServiceAccountArgs{
			Email: pulumi.String(e.DefaultVMServiceAccount()),
			Scopes: pulumi.StringArray{
				pulumi.String("cloud-platform"),
			},
		},
	}, append(opts, e.WithProviders(config.ProviderGCP))...)
	if err != nil {
		return nil, err
	}

	return instance, nil
}
