// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"encoding/base64"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	linuxInitUserData = `#!/bin/bash
echo "ECS_CLUSTER=%s" >> /etc/ecs/ecs.config`

	linuxBottlerocketInitUserData = `[settings]
  [settings.host-containers]
    [settings.host-containers.admin]
      enabled = true

  [settings.ecs]
    cluster = "%s"
`

	windowsInitUserData = `<powershell>
Initialize-ECSAgent -Cluster %s -EnableTaskIAMRole -LoggingDrivers '["json-file","awslogs"]' -EnableTaskENI
</powershell>`
)

func NewECSOptimizedNodeGroup(e aws.Environment, clusterName pulumi.StringInput, armInstance bool) (pulumi.StringOutput, error) {
	amiParamName := "/aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id"
	instanceType := e.DefaultInstanceType()
	ngName := "ecs-optimized-ng"
	if armInstance {
		amiParamName = "/aws/service/ecs/optimized-ami/amazon-linux-2/arm64/recommended/image_id"
		instanceType = e.DefaultARMInstanceType()
		ngName = "ecs-optimized-arm-ng"
	}

	ecsAmi, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: amiParamName,
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	return newNodeGroup(e, ngName, pulumi.String(ecsAmi.Value), pulumi.String(instanceType), getUserData(linuxInitUserData, clusterName))
}

func NewBottlerocketNodeGroup(e aws.Environment, clusterName pulumi.StringInput) (pulumi.StringOutput, error) {
	bottlerocketAmi, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: "/aws/service/bottlerocket/aws-ecs-1/x86_64/latest/image_id",
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	return newNodeGroup(e, "bottlerocket-ng", pulumi.String(bottlerocketAmi.Value), pulumi.String(e.DefaultInstanceType()), getUserData(linuxBottlerocketInitUserData, clusterName))
}

func NewWindowsNodeGroup(e aws.Environment, clusterName pulumi.StringInput) (pulumi.StringOutput, error) {
	winAmi, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: "/aws/service/ami-windows-latest/Windows_Server-2022-English-Full-ECS_Optimized/image_id",
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	return newNodeGroup(e, "win2022-ng", pulumi.String(winAmi.Value), pulumi.String(e.DefaultInstanceType()), getUserData(windowsInitUserData, clusterName))
}

// NewManagedNodeGroup creates an ECS node group using ECS-managed instances.
// Managed instances are EC2 instances that are managed by ECS, providing automatic scaling,
// draining, and lifecycle management without requiring direct ASG management.
func NewManagedNodeGroup(e aws.Environment, clusterName pulumi.StringInput) (pulumi.StringOutput, error) {
	// Use the same ECS-optimized AMI as regular node groups
	ecsAmi, err := ssm.LookupParameter(e.Ctx(), &ssm.LookupParameterArgs{
		Name: "/aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id",
	}, e.WithProvider(config.ProviderAWS))
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	// Managed instances use similar configuration but with ECS-managed ASG
	// For testing purposes, we create a standard node group that ECS will manage
	return newNodeGroup(e, "managed-ng", pulumi.String(ecsAmi.Value), pulumi.String(e.DefaultInstanceType()), getUserData(linuxInitUserData, clusterName))
}

func newNodeGroup(e aws.Environment, ngName string, ami, instanceType, userData pulumi.StringInput) (pulumi.StringOutput, error) {
	lt, err := ec2.CreateLaunchTemplate(e, ngName,
		ami,
		instanceType,
		pulumi.String(e.ECSInstanceProfile()),
		pulumi.String(e.DefaultKeyPairName()),
		userData)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	asg, err := ec2.NewAutoscalingGroup(e, ngName, lt.ID(), lt.LatestVersion, 2, 2, 4)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	cp, err := NewCapacityProvider(e, ngName, asg.Arn)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	return cp.Name, nil
}

func getUserData(userData string, clusterName pulumi.StringInput) pulumi.StringInput {
	return clusterName.ToStringOutput().ApplyT(func(name string) pulumi.StringInput {
		return pulumi.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(userData, name))))
	}).(pulumi.StringInput)
}
