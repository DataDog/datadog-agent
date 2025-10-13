package ec2

import (
	"strconv"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewAutoscalingGroup(e aws.Environment, name string,
	launchTemplateID pulumi.StringInput,
	launchTemplateVersion pulumi.IntInput,
	desiredCapacity, minSize, maxSize int,
) (*autoscaling.Group, error) {
	return autoscaling.NewGroup(e.Ctx(), e.Namer.ResourceName(name), &autoscaling.GroupArgs{
		NamePrefix:      e.CommonNamer().DisplayName(255, pulumi.String(name)),
		DesiredCapacity: pulumi.Int(desiredCapacity),
		MinSize:         pulumi.Int(minSize),
		MaxSize:         pulumi.Int(maxSize),
		LaunchTemplate: autoscaling.GroupLaunchTemplateArgs{
			Id:      launchTemplateID,
			Version: launchTemplateVersion.ToIntOutput().ApplyT(func(v int) pulumi.String { return pulumi.String(strconv.Itoa(v)) }).(pulumi.StringInput),
		},
		CapacityRebalance: pulumi.Bool(true),
		InstanceRefresh: autoscaling.GroupInstanceRefreshArgs{
			Strategy: pulumi.String("Rolling"),
			Preferences: autoscaling.GroupInstanceRefreshPreferencesArgs{
				MinHealthyPercentage: pulumi.Int(0),
			},
		},
	}, e.WithProviders(config.ProviderAWS))
}
