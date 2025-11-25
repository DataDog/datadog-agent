// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package npmtools

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	ecsComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/awsx"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type EcsComponent struct {
	pulumi.ResourceState
}

func EcsAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, testURL string, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("npm-tools")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsComponent))

	if _, err := ecs.NewEC2Service(e.Ctx(), namer.ResourceName("curl-dig"), &ecs.EC2ServiceArgs{
		Name:                 e.CommonNamer().DisplayName(255, pulumi.String("curl-dig")),
		Cluster:              clusterArn,
		DesiredCount:         pulumi.IntPtr(1),
		EnableExecuteCommand: pulumi.BoolPtr(true),
		TaskDefinitionArgs: &ecs.EC2ServiceTaskDefinitionArgs{
			Containers: map[string]ecs.TaskDefinitionContainerDefinitionArgs{
				"curl-dig": {
					Name:  pulumi.String("curl-dig"),
					Image: pulumi.String("ghcr.io/datadog/apps-npm-tools:" + apps.Version),
					Command: pulumi.StringArray{
						pulumi.String("sh"),
						pulumi.String("-c"),
						pulumi.String(fmt.Sprintf("while [ 1 ] ; do curl %s ; dig @8.8.8.8 www.google.ch ; sleep 20 ; done", testURL)),
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
			Family:      e.CommonNamer().DisplayName(255, pulumi.String("curl-dig-ec2")),
		},
	}, opts...); err != nil {
		return nil, err
	}

	return ecsComponent, nil
}
