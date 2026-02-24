// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewCapacityProvider(e aws.Environment, name string, asgArn pulumi.StringInput) (*ecs.CapacityProvider, error) {
	return ecs.NewCapacityProvider(e.Ctx(), e.Namer.ResourceName(name), &ecs.CapacityProviderArgs{
		Name: e.CommonNamer().DisplayName(255, pulumi.String(name)),
		AutoScalingGroupProvider: &ecs.CapacityProviderAutoScalingGroupProviderArgs{
			AutoScalingGroupArn: asgArn,
			ManagedScaling: &ecs.CapacityProviderAutoScalingGroupProviderManagedScalingArgs{
				Status: aws.DisabledString,
			},
			ManagedTerminationProtection: aws.DisabledString,
		},
	}, e.WithProviders(config.ProviderAWS))
}

func NewClusterCapacityProvider(e aws.Environment, name string, clusterName pulumi.StringInput, capacityProviders pulumi.StringArray) (*ecs.ClusterCapacityProviders, error) {
	return ecs.NewClusterCapacityProviders(e.Ctx(), e.Namer.ResourceName(name), &ecs.ClusterCapacityProvidersArgs{
		ClusterName:       clusterName,
		CapacityProviders: capacityProviders,
	}, e.WithProviders(config.ProviderAWS))
}
