package ec2

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

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
