package cpustress

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	ecsClient "github.com/DataDog/test-infra-definitions/resources/aws/ecs"
	classicECS "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func FargateAppDefinition(e aws.Environment, clusterArn pulumi.StringInput, apiKeySSMParamName pulumi.StringInput, fakeIntake *fakeintakeComp.Fakeintake, opts ...pulumi.ResourceOption) (*ecsComp.Workload, error) {
	namer := e.Namer.WithPrefix("cpustress").WithPrefix("fg")
	opts = append(opts, e.WithProviders(config.ProviderAWS, config.ProviderAWSX))

	ecsFargateComponent := &ecsComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", namer.ResourceName("grp"), ecsFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ecsFargateComponent))

	stressContainer := &ecs.TaskDefinitionContainerDefinitionArgs{
		Name:  pulumi.String("stress-ng"),
		Image: pulumi.String("ghcr.io/datadog/apps-stress-ng:" + apps.Version),
		DockerLabels: pulumi.StringMap{
			"com.datadoghq.ad.tags": pulumi.String("[\"ecs_launch_type:fargate\"]"),
		},
		Command: pulumi.StringArray{
			pulumi.String("--cpu=1"),
			pulumi.String("--cpu-load=15"),
		},
		Cpu:    pulumi.IntPtr(200),
		Memory: pulumi.IntPtr(64),
	}

	stressTaskDef, err := ecsClient.FargateTaskDefinitionWithAgent(e, "stress-ng-fg", pulumi.String("stress-ng-fg"), 1024, 2048,
		map[string]ecs.TaskDefinitionContainerDefinitionArgs{
			"stress-ng": *stressContainer,
		},
		apiKeySSMParamName,
		fakeIntake,
		"",
		opts...,
	)
	if err != nil {
		return nil, err
	}

	if _, err := ecs.NewFargateService(e.Ctx(), namer.ResourceName("stress-ng"), &ecs.FargateServiceArgs{
		Name:         e.CommonNamer().DisplayName(255, pulumi.String("stress-ng"), pulumi.String("fg")),
		Cluster:      clusterArn,
		DesiredCount: pulumi.IntPtr(1),
		NetworkConfiguration: classicECS.ServiceNetworkConfigurationArgs{
			AssignPublicIp: pulumi.BoolPtr(e.ECSServicePublicIP()),
			SecurityGroups: pulumi.ToStringArray(e.DefaultSecurityGroups()),
			Subnets:        e.RandomSubnets(),
		},
		TaskDefinition:            stressTaskDef.TaskDefinition.Arn(),
		EnableExecuteCommand:      pulumi.BoolPtr(true),
		ContinueBeforeSteadyState: pulumi.BoolPtr(true),
	}, opts...); err != nil {
		return nil, err
	}

	return ecsFargateComponent, nil
}
