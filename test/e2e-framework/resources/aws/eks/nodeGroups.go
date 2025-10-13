package eks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/resources/aws"

	awsEc2 "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awsEks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	amazonLinux2AMD64AmiType    = "AL2_x86_64"
	amazonLinux2ARM64AmiType    = "AL2_ARM_64"
	amazonLinux2023AMD64AmiType = "AL2023_x86_64_STANDARD"
	amazonLinux2023ARM64AmiType = "AL2023_ARM_64_STANDARD"
	bottlerocketAmiType         = "BOTTLEROCKET_x86_64"
	windowsAmiType              = "WINDOWS_CORE_2022_x86_64"
)

func NewAL2023LinuxNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux"
	lt, err := newLinuxLaunchTemplate(e, name+"-launch-template", opts...)
	if err != nil {
		return nil, err
	}
	return newManagedNodeGroup(e, name, cluster, nodeRole, amazonLinux2023AMD64AmiType, e.DefaultInstanceType(), lt, opts...)

}

func NewAL2023LinuxARMNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	name := "linux-arm"
	lt, err := newLinuxLaunchTemplate(e, name+"-launch-template", opts...)

	if err != nil {
		return nil, err
	}

	return newManagedNodeGroup(e, name, cluster, nodeRole, amazonLinux2023ARM64AmiType, e.DefaultARMInstanceType(), lt, opts...)
}

func NewLinuxNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "linux", cluster, nodeRole, amazonLinux2AMD64AmiType, e.DefaultInstanceType(), nil, opts...)
}

func NewLinuxARMNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "linux-arm", cluster, nodeRole, amazonLinux2ARM64AmiType, e.DefaultARMInstanceType(), nil, opts...)
}

func NewBottlerocketNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "bottlerocket", cluster, nodeRole, bottlerocketAmiType, e.DefaultInstanceType(), nil, opts...)
}

func NewWindowsNodeGroup(e aws.Environment, cluster *eks.Cluster, nodeRole *awsIam.Role, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
	return newManagedNodeGroup(e, "windows", cluster, nodeRole, windowsAmiType, e.DefaultInstanceType(), nil, opts...)
}

func newLinuxLaunchTemplate(e aws.Environment, name string, opts ...pulumi.ResourceOption) (*awsEc2.LaunchTemplate, error) {
	nodeGroupInstanceSG, err := awsEc2.NewSecurityGroup(e.Ctx(), e.Namer.ResourceName(name+"-security-group"),
		&awsEc2.SecurityGroupArgs{
			Description: pulumi.String("Security group for all nodes in the nodeGroup to allow SSH access"),
			VpcId:       pulumi.StringPtr(e.DefaultVPCID()),
			Ingress: awsEc2.SecurityGroupIngressArray{
				&awsEc2.SecurityGroupIngressArgs{
					CidrBlocks:     pulumi.ToStringArray(e.EKSAllowedInboundCIDRs()),
					SecurityGroups: pulumi.ToStringArray(e.EKSAllowedInboundSecurityGroups()),
					PrefixListIds:  pulumi.ToStringArray(e.EKSAllowedInboundPrefixLists()),
					ToPort:         pulumi.Int(22),
					FromPort:       pulumi.Int(22),
					Protocol:       pulumi.String("tcp"),
				},
			},
			// Egress to internet, both IPV4 and IPV6
			Egress: awsEc2.SecurityGroupEgressArray{
				&awsEc2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
				&awsEc2.SecurityGroupEgressArgs{
					Protocol:       pulumi.String("-1"),
					FromPort:       pulumi.Int(0),
					ToPort:         pulumi.Int(0),
					Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
				},
			},
		}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS, config.ProviderEKS))...)

	if err != nil {
		return nil, fmt.Errorf("error creating SSH access security group: %v", err)
	}

	return awsEc2.NewLaunchTemplate(e.Ctx(), name, &awsEc2.LaunchTemplateArgs{
		UpdateDefaultVersion: pulumi.BoolPtr(true),
		KeyName:              pulumi.String(e.DefaultKeyPairName()),
		MetadataOptions: &awsEc2.LaunchTemplateMetadataOptionsArgs{
			HttpPutResponseHopLimit: pulumi.IntPtr(2),
		},
		BlockDeviceMappings: awsEc2.LaunchTemplateBlockDeviceMappingArray{
			&awsEc2.LaunchTemplateBlockDeviceMappingArgs{
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
		VpcSecurityGroupIds: append(pulumi.StringArray{nodeGroupInstanceSG.ID()},
			pulumi.ToStringArray(e.DefaultSecurityGroups())...),
	}, utils.MergeOptions(opts, e.WithProviders(config.ProviderAWS, config.ProviderEKS))...)
}

func newManagedNodeGroup(e aws.Environment, name string, cluster *eks.Cluster, nodeRole *awsIam.Role, amiType, instanceType string, launchTemplate *awsEc2.LaunchTemplate, opts ...pulumi.ResourceOption) (*eks.ManagedNodeGroup, error) {
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

	// common args
	args := &eks.ManagedNodeGroupArgs{
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
