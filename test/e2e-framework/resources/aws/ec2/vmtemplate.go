// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateLaunchTemplate creates an EC2 launch template with the given parameters.
func CreateLaunchTemplate(e aws.Environment, name string, ami, instanceType, iamProfileArn, keyPair, userData pulumi.StringInput) (*ec2.LaunchTemplate, error) {
	launchTemplate, err := ec2.NewLaunchTemplate(e.Ctx(), e.Namer.ResourceName(name), &ec2.LaunchTemplateArgs{
		ImageId:      ami,
		NamePrefix:   e.CommonNamer().DisplayName(128, pulumi.String(name)),
		InstanceType: instanceType,
		IamInstanceProfile: ec2.LaunchTemplateIamInstanceProfileArgs{
			Arn: iamProfileArn,
		},
		NetworkInterfaces: ec2.LaunchTemplateNetworkInterfaceArray{
			ec2.LaunchTemplateNetworkInterfaceArgs{
				SubnetId:                 e.RandomSubnets().Index(pulumi.Int(0)),
				DeleteOnTermination:      pulumi.StringPtr("true"),
				AssociatePublicIpAddress: pulumi.StringPtr("false"),
				SecurityGroups:           pulumi.ToStringArray(e.DefaultSecurityGroups()),
			},
		},
		BlockDeviceMappings: ec2.LaunchTemplateBlockDeviceMappingArray{
			ec2.LaunchTemplateBlockDeviceMappingArgs{},
		},
		KeyName:              keyPair,
		UserData:             userData,
		UpdateDefaultVersion: pulumi.BoolPtr(true),
	}, e.WithProviders(config.ProviderAWS))
	return launchTemplate, err
}
