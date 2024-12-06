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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"

	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go/aws"
)

type myECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestMyECSSuite(t *testing.T) {
	e2e.Run(t, &myECSSuite{}, e2e.WithProvisioner(ecs.Provisioner(ecs.WithECSOptions(tifEcs.WithLinuxNodeGroup()))))
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
