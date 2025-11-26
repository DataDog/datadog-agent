// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package cpustress

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type EcsComponent struct {
	pulumi.ResourceState
}

func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("cpustress").WithPrefix("ec2")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("stress-ng"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("stress-ng")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"stress-ng": {
					Name:  pulumi.String("stress-ng"),
					Image: pulumi.String("ghcr.io/datadog/apps-stress-ng:" + apps.Version),
					Command: pulumi.StringArray{
						pulumi.String("--cpu=1"),
						pulumi.String("--cpu-load=15"),
					},
					Cpu:    pulumi.IntPtr(200),
					Memory: pulumi.IntPtr(64),
				},
			},
			ExecutionRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskExecutionRole()),
			},
			TaskRole: &awsx.DefaultRoleWithPolicyArgs{
				RoleArn: pulumi.StringPtr(e.ECSTaskRole()),
			},
			NetworkMode: pulumi.StringPtr("none"),
			Family:      e.CommonNamer().DisplayName(255, pulumi.String("stress-ng-ec2")),
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
