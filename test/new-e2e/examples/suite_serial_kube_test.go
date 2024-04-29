// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/kindvm"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type myEnv struct {
	kind *components.KubernetesCluster `import:"dd-KubernetesCluster-kind"`
}

type mySuite struct {
	e2e.BaseSuite[myEnv]
}

func TestMySuite(t *testing.T) {
	e2e.Run(t, &mySuite{}, e2e.WithUntypedPulumiProvisioner(kindvm.Run, runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "false"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "false"},
	}))
}

func (s *mySuite) TestRun() {
	s.Assert().NotNil(s.Env().kind.Client())
}
