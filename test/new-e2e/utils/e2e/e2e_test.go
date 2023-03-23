// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	ec2vm "github.com/DataDog/test-infra-definitions/aws/scenarios/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/vm/os"
	commonos "github.com/DataDog/test-infra-definitions/common/os"
	"github.com/DataDog/test-infra-definitions/datadog/agent"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MyEnv struct {
	VM    *client.VM
	Agent *client.Agent
}

type e2eSuite struct {
	*Suite[MyEnv]
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, &e2eSuite{Suite: NewSuite("my-test", &StackDefinition[MyEnv]{
		EnvCloudName: "aws/sandbox",
		EnvFactory: func(ctx *pulumi.Context) (*MyEnv, error) {
			vm, err := ec2vm.NewUnixEc2VM(ctx, ec2vm.WithOS(os.AmazonLinuxOS, commonos.AMD64Arch))
			if err != nil {
				return nil, err
			}

			installer, err := agent.NewInstaller(vm)
			if err != nil {
				return nil, err
			}
			return &MyEnv{
				VM:    client.NewVM(vm),
				Agent: client.NewAgent(installer),
			}, nil
		},
	})})
}

func (v *e2eSuite) TestVM() {
	output, err := v.Env.VM.Execute("ls")
	require.NoError(v.T(), err)
	require.NotEmpty(v.T(), output)
}

func (v *e2eSuite) TestAgent() {
	output, err := v.Env.Agent.Status()
	require.NoError(v.T(), err)
	require.Contains(v.T(), output, "Agent start")
}
