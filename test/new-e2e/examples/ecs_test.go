// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
)

type myECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestMyECSSuite(t *testing.T) {
	e2e.Run(t, &myECSSuite{}, e2e.WithProvisioner(ecs.Provisioner(ecs.WithRunOptions(scenecs.WithECSOptions(scenecs.WithLinuxNodeGroup())))))
}

func (v *myECSSuite) TestECS() {
	ctx := context.Background()

	tasks, err := v.Env().ECSCluster.ECSClient.ListTasks(ctx, &awsecs.ListTasksInput{
		Cluster: aws.String(v.Env().ECSCluster.ClusterName),
	})
	require.NoError(v.T(), err)
	require.NotEmpty(v.T(), tasks.TaskArns)
	out, err := v.Env().ECSCluster.ECSClient.ExecCommand(tasks.TaskArns[0], "datadog-agent", "ls -l")
	require.NoError(v.T(), err)
	fmt.Println("cmd output:", out, "end of cmd output")
	require.NotEmpty(v.T(), out)
}
