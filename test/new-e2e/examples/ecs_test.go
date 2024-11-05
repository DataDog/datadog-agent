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

	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
)

type myECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestMyECSSuite(t *testing.T) {
	e2e.Run(t, &myECSSuite{}, e2e.WithProvisioner(ecs.Provisioner(ecs.WithECSOptions(tifEcs.WithLinuxNodeGroup()))))
}

func (v *myECSSuite) TestECS() {
	ctx := context.Background()
	services, err := v.Env().ECSCluster.ECSClient.ListServices(ctx, &awsecs.ListServicesInput{})
	v.Require().NoError(err)
	for _, service := range services.ServiceArns {
		fmt.Println("Service:", service)
	}
	fmt.Println("Services:", services)

}
