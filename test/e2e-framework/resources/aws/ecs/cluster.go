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

func CreateEcsCluster(e aws.Environment, name string) (*ecs.Cluster, error) {
	return ecs.NewCluster(e.Ctx(), e.Namer.ResourceName(name), &ecs.ClusterArgs{
		Name: e.CommonNamer().DisplayName(255, pulumi.String(name)),
		Configuration: &ecs.ClusterConfigurationArgs{
			ExecuteCommandConfiguration: &ecs.ClusterConfigurationExecuteCommandConfigurationArgs{
				KmsKeyId: pulumi.StringPtr(e.ECSExecKMSKeyID()),
			},
		},
	}, e.WithProviders(config.ProviderAWS))
}
