// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/vboulineau/pulumi-definitions/aws"
	"github.com/vboulineau/pulumi-definitions/aws/ecs/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"

	datadog "gopkg.in/zorkian/go-datadog-api.v2"
)

func TestAgentOnECS(t *testing.T) {
	config := auto.ConfigMap{}
	env := aws.NewSandboxEnvironment(config)
	credentialsManager := credentials.NewManager()

	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(t, err)
	appKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.appkey")
	require.NoError(t, err)

	stack, err := infra.NewStack(context.Background(), "ci", "ecs-fargate", config, func(ctx *pulumi.Context) error {
		return ecs.Run(ctx, env)
	})
	require.NoError(t, err)
	require.NotNil(t, stack)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	result, err := stack.Up(ctx)
	cancel()
	require.NoError(t, err)

	// ctx.Export("ecs-cluster-name", ecsCluster.Name)
	// ctx.Export("ecs-cluster-arn", ecsCluster.Arn)
	// ctx.Export("ecs-task-arn", taskDef.TaskDefinition.Arn())
	// ctx.Export("ecs-task-family", taskDef.TaskDefinition.Family())
	// ctx.Export("ecs-task-version", taskDef.TaskDefinition.Revision())

	ecsClusterName := result.Outputs["ecs-cluster-name"].Value.(string)
	ecsTaskFamily := result.Outputs["ecs-task-family"].Value.(string)
	ecsTaskVersion := result.Outputs["ecs-task-version"].Value.(float64)

	// Check content in Datadog
	datadogClient := datadog.NewClient(apiKey, appKey)
	query := fmt.Sprintf("avg:ecs.fargate.cpu.user{ecs_cluster_name:%s,ecs_task_family:%s,ecs_task_version:%.0f} by {ecs_container_name}", ecsClusterName, ecsTaskFamily, ecsTaskVersion)
	t.Log(query)

	err = backoff.Retry(func() error {
		currentTime := time.Now().Unix()
		series, err := datadogClient.QueryMetrics(currentTime-120, currentTime, query)
		if err != nil {
			return err
		}

		if len(series) == 0 {
			return errors.New("No data yet")
		}

		if len(series) != 3 {
			return errors.New("Not all containers")
		}

		if *series[0].Points[0][1] == 0 {
			return errors.New("0-value")
		}

		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30), 6))
	require.NoError(t, err)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	err = stack.Down(ctx)
	cancel()
	require.NoError(t, err)
}
