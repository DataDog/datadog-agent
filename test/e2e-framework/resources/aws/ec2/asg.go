// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"strconv"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewAutoscalingGroup creates an EC2 autoscaling group with the given parameters.
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
