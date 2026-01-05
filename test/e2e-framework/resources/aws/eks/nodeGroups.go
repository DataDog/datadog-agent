// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	awsEc2 "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewAL2023LinuxNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux"
	lt, err := newAL2023LaunchTemplate(e, name+"-launch-template", opts...)
	if err != nil {
		return nil, err
	}
	descriptor := os.NewDescriptorWithArch(os.AmazonLinuxEKS, fmt.Sprintf("%s-%s", e.KubernetesVersion(), "al2023"), os.AMD64Arch)
	amiId, err := aws.GetAMI(&descriptor)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, name, cluster, nodeRole, amiId, e.DefaultInstanceType(), false, lt, opts...)

}

func NewAL2023LinuxARMNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux-arm"
	lt, err := newAL2023LaunchTemplate(e, name+"-launch-template", opts...)

	if err != nil {
		return nil, err
	}
	descriptor := os.NewDescriptorWithArch(os.AmazonLinuxEKS, fmt.Sprintf("%s-%s", e.KubernetesVersion(), "al2023"), os.ARM64Arch)
	amiId, err := aws.GetAMI(&descriptor)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, name, cluster, nodeRole, amiId, e.DefaultARMInstanceType(), false, lt, opts...)
}

func NewBottlerocketNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	descriptor := os.NewDescriptorWithArch(os.BottlerocketEKS, e.KubernetesVersion(), os.AMD64Arch)
	amiId, err := aws.GetAMI(&descriptor)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, "bottlerocket", cluster, nodeRole, amiId, e.DefaultInstanceType(), false, nil, opts...)
}

func NewWindowsNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	descriptor := os.NewDescriptorWithArch(os.WindowsServerEKS, fmt.Sprintf("%s-%s", e.KubernetesVersion(), "server-2022"), os.AMD64Arch)
	amiId, err := aws.GetAMI(&descriptor)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, "windows", cluster, nodeRole, amiId, e.DefaultInstanceType(), true, nil, opts...)
}

func newAL2023LaunchTemplate(e aws.Environment, name string, opts ...pulumi.ResourceOption) (*awsEc2.LaunchTemplate, error) {
	prefixLists := make([]string, 0, len(e.EKSAllowedInboundManagedPrefixListNames()))
	for _, plName := range e.EKSAllowedInboundManagedPrefixListNames() {
		pl, err := awsEc2.LookupManagedPrefixList(e.Ctx(), &awsEc2.LookupManagedPrefixListArgs{
			Name: &plName,
		}, e.WithProvider(config.ProviderAWS))
		if err != nil {
			return nil, err
		}
		if pl != nil {
			prefixLists = append(prefixLists, pl.Id)
		}
	}

	sshSG, err := awsEc2.NewSecurityGroup(e.Ctx(), e.Namer.ResourceName(name+"-remote-access-sg"), &awsEc2.SecurityGroupArgs{
		Description: pulumi.StringPtr("Security group for all nodes in the nodeGroup to allow direct SSH access"),
		Ingress: awsEc2.SecurityGroupIngressArray{
			awsEc2.SecurityGroupIngressArgs{
				SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
				PrefixListIds:  pulumi.ToStringArray(append(e.EKSAllowedInboundPrefixLists(), prefixLists...)),
				ToPort:         pulumi.Int(22),
				FromPort:       pulumi.Int(22),
				Protocol:       pulumi.String("tcp"),
			},
		},
		VpcId: pulumi.StringPtr(e.DefaultVPCID()),
	}, e.WithProviders(config.ProviderAWS))
	if err != nil {
		return nil, err
	}

	return awsEc2.NewLaunchTemplate(e.Ctx(), name, &awsEc2.LaunchTemplateArgs{
		UpdateDefaultVersion: pulumi.BoolPtr(true),
		KeyName:              pulumi.String(e.DefaultKeyPairName()),
		MetadataOptions: &awsEc2.LaunchTemplateMetadataOptionsArgs{
			HttpPutResponseHopLimit: pulumi.IntPtr(2),
		},
		BlockDeviceMappings: awsEc2.LaunchTemplateBlockDeviceMappingArray{
			&awsEc2.LaunchTemplateBlockDeviceMappingArgs{
				/*
					aws ssm get-parameter --name /aws/service/eks/optimized-ami/1.30/amazon-linux-2023/x86_64/standard/recommended/image_id --query "Parameter.Value" --output text
					ami-0cd798eab7ada4d4d

					 aws ec2 describe-images --image-ids ami-0cd798eab7ada4d4d   --query 'Images[0].RootDeviceName'
					"/dev/xvda"
				*/
				DeviceName: pulumi.String("/dev/xvda"),
				Ebs: &awsEc2.LaunchTemplateBlockDeviceMappingEbsArgs{
					VolumeSize:          pulumi.Int(80),
					VolumeType:          pulumi.String("gp3"),
					DeleteOnTermination: pulumi.String("true"),
					Encrypted:           pulumi.String("false"),
				},
			},
		},
		// Attach the SSH access Security Group created above, as well as the default security groups.
		// This is done to replicate what EKS does behind the scenes when you don't specify a launch template
		VpcSecurityGroupIds: append(pulumi.StringArray{sshSG.ID()}, pulumi.ToStringArray(e.DefaultSecurityGroups())...),
	}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS, config.ProviderEKS))...)
}

func newManagedNodeGroup(e aws.Environment, name string, cluster *eks.Cluster, nodeRole *awsIam.Role, amiId, instanceType string, isWindows bool, launchTemplate *awsEc2.LaunchTemplate, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	taints := awsEks.NodeGroupTaintArray{}
	if isWindows {
		taints = append(taints,
			awsEks.NodeGroupTaintArgs{
				Key:    pulumi.String("node.kubernetes.io/os"),
				Value:  pulumi.String("windows"),
				Effect: pulumi.String("NO_SCHEDULE"),
			},
		)
	}

	// common args
	args := &eks.ManagedNodeGroupArgs{
		AmiId:               pulumi.StringPtr(amiId),
		Cluster:             cluster.Core,
		InstanceTypes:       pulumi.ToStringArray([]string{instanceType}),
		ForceUpdateVersion:  pulumi.BoolPtr(true),
		NodeGroupNamePrefix: e.CommonNamer().DisplayName(37, pulumi.String(name), pulumi.String("ng")),
		ScalingConfig: awsEks.NodeGroupScalingConfigArgs{
			DesiredSize: pulumi.Int(1),
			MaxSize:     pulumi.Int(1),
			MinSize:     pulumi.Int(0),
		},
		NodeRole: nodeRole,
		Taints:   taints,
	}

	if launchTemplate != nil {
		args.LaunchTemplate = &awsEks.NodeGroupLaunchTemplateArgs{
			Id:      launchTemplate.ID(),
			Version: launchTemplate.DefaultVersion.ApplyT(func(v int) pulumi.String { return pulumi.String(strconv.Itoa(v)) }).(pulumi.StringInput),
		}
	} else {
		args.DiskSize = pulumi.Int(80)
		args.RemoteAccess = awsEks.NodeGroupRemoteAccessArgs{
			Ec2SshKey:              pulumi.String(e.DefaultKeyPairName()),
			SourceSecurityGroupIds: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
		}
	}

	return eks.NewManagedNodeGroup(e.Ctx(), e.Namer.ResourceName(name), args, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS, config.ProviderEKS))...)
}
