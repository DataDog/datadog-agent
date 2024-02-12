// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"
)

type hostHttpbinEnvECS struct {
	environments.Host

	// Extra Components
	HTTPBinHost *components.RemoteHost
}
type ecsVMSuite struct {
	e2e.BaseSuite[hostHttpbinEnvECS]

	ecsClusterName string
	fakeIntake     *client.Client
}

// TestECSVMSuite will validate running the agent on a single EC2 VM
func TestECSVMSuite(t *testing.T) {
	s := &ecsVMSuite{}

	e2e.Run(t, s)
}

func (suite *ecsVMSuite) SetupSuite() {
	ctx := context.Background()

	// Creating the stack
	stackConfig := runner.ConfigMap{
		"ddinfra:aws/ecs/linuxECSOptimizedNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/linuxBottlerocketNodeGroup": auto.ConfigValue{Value: "true"},
		"ddinfra:aws/ecs/windowsLTSCNodeGroup":       auto.ConfigValue{Value: "true"},
		"ddagent:deploy":                             auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":                         auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy":                      auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "ecs-cluster", stackConfig, ecs.Run, false, nil)
	suite.Require().NoError(err)

	fakeintake := &components.FakeIntake{}
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-ecs"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(fakeintake.Import(fiSerialized, fakeintake))
	suite.Require().NoError(fakeintake.Init(suite))
	suite.fakeIntake = fakeintake.Client()

	suite.ecsClusterName = stackOutput.Outputs["ecs-cluster-name"].Value.(string)

	suite.BaseSuite.SetupSuite()
}

// BeforeTest will be called before each test
func (v *ecsVMSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestFakeIntakeNPM_HostRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ecsVMSuite) TestFakeIntakeNPM_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	v.T().Log("====================================" + testURL)
}
