// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type myECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestMyECSSuite(t *testing.T) {
	e2e.Run(t, &myECSSuite{}, e2e.WithProvisioner(ecs.Provisioner(ecs.WithECSOptions(tifEcs.WithLinuxNodeGroup()), ecs.WithWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error) {
		return redis.EcsAppDefinition(e, clusterArn)
	}))))
}

func (v *myECSSuite) TestECS() {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	v.Require().NoError(err)

	client := awsecs.NewFromConfig(cfg)
	services, err := client.ListServices(ctx, &awsecs.ListServicesInput{})
	v.Require().NoError(err)

	for _, service := range services.ServiceArns {
		fmt.Println("Service:", service)
	}
	fmt.Println("Services:", services)

}
