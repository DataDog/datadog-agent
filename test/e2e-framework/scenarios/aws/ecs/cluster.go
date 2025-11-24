// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewCluster(e aws.Environment, name string, opts ...Option) (*ecsComp.Cluster, error) {
	params, err := NewParams(opts...)
	if err != nil {
		return nil, err
	}

	return components.NewComponent(&e, name, func(comp *ecsComp.Cluster) error {
		ecsCluster, err := ecs.CreateEcsCluster(e, name)
		if err != nil {
			return err
		}

		comp.ClusterArn = ecsCluster.Arn
		comp.ClusterName = ecsCluster.Name

		// Handle capacity providers
		capacityProviders := pulumi.StringArray{}
		if params.FargateCapacityProvider {
			capacityProviders = append(capacityProviders, pulumi.String("FARGATE"))
		}

		if params.LinuxNodeGroup {
			cpName, err := ecs.NewECSOptimizedNodeGroup(e, ecsCluster.Name, false)
			if err != nil {
				return err
			}

			capacityProviders = append(capacityProviders, cpName)
		}

		if params.LinuxARMNodeGroup {
			cpName, err := ecs.NewECSOptimizedNodeGroup(e, ecsCluster.Name, true)
			if err != nil {
				return err
			}

			capacityProviders = append(capacityProviders, cpName)
		}

		if params.LinuxBottleRocketNodeGroup {
			cpName, err := ecs.NewBottlerocketNodeGroup(e, ecsCluster.Name)
			if err != nil {
				return err
			}

			capacityProviders = append(capacityProviders, cpName)
		}

		if params.WindowsNodeGroup {
			cpName, err := ecs.NewWindowsNodeGroup(e, ecsCluster.Name)
			if err != nil {
				return err
			}

			capacityProviders = append(capacityProviders, cpName)
		}

		// Associate capacity providers
		_, err = ecs.NewClusterCapacityProvider(e, e.Ctx().Stack(), ecsCluster.Name, capacityProviders)
		if err != nil {
			return err
		}

		return nil
	})
}
