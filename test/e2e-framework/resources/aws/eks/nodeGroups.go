// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	awsEc2 "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	amazonLinux2023AMD64AmiType      = "AL2023_x86_64_STANDARD"
	amazonLinux2023ARM64AmiType      = "AL2023_ARM_64_STANDARD"
	amazonLinux2023NVIDIAGPUAmiType  = "AL2023_x86_64_NVIDIA"
	bottlerocketAmiType              = "BOTTLEROCKET_x86_64"
	windowsAmiType                   = "WINDOWS_CORE_2022_x86_64"
)

func NewAL2023LinuxNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux"
	lt, err := newAL2023LaunchTemplate(e, name+"-launch-template", opts...)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, name, cluster, nodeRole, amazonLinux2023AMD64AmiType, e.DefaultInstanceType(), lt, opts...)

}

func NewAL2023LinuxARMNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux-arm"
	lt, err := newAL2023LaunchTemplate(e, name+"-launch-template", opts...)

	if err != nil {
		return nil, err
	}

	return newManagedNodeGroup(e, name, cluster, nodeRole, amazonLinux2023ARM64AmiType, e.DefaultARMInstanceType(), lt, opts...)
}

func NewBottlerocketNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "bottlerocket", cluster, nodeRole, bottlerocketAmiType, e.DefaultInstanceType(), nil, opts...)
}

func NewWindowsNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "windows", cluster, nodeRole, windowsAmiType, e.DefaultInstanceType(), nil, opts...)
}

// NewGPULinuxNodeGroup creates a GPU-enabled Linux node group using Amazon Linux 2023 with NVIDIA GPU support.
// The node group uses the AL2023_x86_64_NVIDIA AMI which has pre-installed NVIDIA drivers.
// Note: AL2_x86_64_GPU is only supported for Kubernetes 1.32 or earlier, so we use AL2023_x86_64_NVIDIA for K8s 1.33+.
func NewGPULinuxNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, instanceType string, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux-gpu"
	lt, err := newAL2023LaunchTemplate(e, name+"-launch-template", opts...)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{
		// Mimic NFD (Node Feature Discovery) label for GPU nodes
		// This allows the NVIDIA device plugin to use its default affinity
		"nvidia.com/gpu.present": "true",
	}
	return newManagedNodeGroupWithLabels(e, name, cluster, nodeRole, amazonLinux2023NVIDIAGPUAmiType, instanceType, lt, labels, opts...)
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

func newManagedNodeGroup(e aws.Environment, name string, cluster *eks.Cluster, nodeRole *awsIam.Role, amiType, instanceType string, launchTemplate *awsEc2.LaunchTemplate, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroupWithLabels(e, name, cluster, nodeRole, amiType, instanceType, launchTemplate, nil, opts...)
}

// newManagedNodeGroupWithLabels creates a managed node group with optional node labels.
func newManagedNodeGroupWithLabels(e aws.Environment, name string, cluster *eks.Cluster, nodeRole *awsIam.Role, amiType, instanceType string, launchTemplate *awsEc2.LaunchTemplate, labels map[string]string, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	taints := awsEks.NodeGroupTaintArray{}
	if strings.Contains(amiType, "WINDOWS") {
		taints = append(taints,
			awsEks.NodeGroupTaintArgs{
				Key:    pulumi.String("node.kubernetes.io/os"),
				Value:  pulumi.String("windows"),
				Effect: pulumi.String("NO_SCHEDULE"),
			},
		)
	}

	releaseVersion, err := GetNodesVersion(amiType, e.KubernetesVersion())
	if err != nil {
		return nil, err
	}

	// Convert labels to pulumi.StringMap
	pulumiLabels := pulumi.StringMap{}
	for k, v := range labels {
		pulumiLabels[k] = pulumi.String(v)
	}

	// common args
	args := &eks.ManagedNodeGroupArgs{
		ReleaseVersion:      pulumi.String(releaseVersion),
		AmiType:             pulumi.StringPtr(amiType),
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
		Labels:   pulumiLabels,
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
