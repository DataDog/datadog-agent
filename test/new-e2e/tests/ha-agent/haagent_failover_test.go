// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	componentsOs "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type multiVMEnv struct {
	Host1  *components.RemoteHost
	Host2  *components.RemoteHost
	Agent1 *components.RemoteHostAgent
	Agent2 *components.RemoteHostAgent
	API    *datadogV1.MetricsApi
}

func multiVMEnvProvisioner() provisioners.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {

		// language=yaml
		agentConfig1 := `
hostname: test-e2e-agent1
ha_agent:
    enabled: true
config_id: test-e2e-config-id
log_level: debug
`

		agentConfig2 := `
hostname: test-e2e-agent2
ha_agent:
    enabled: true
config_id: test-e2e-config-id
log_level: debug
`

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host1, err := ec2.NewVM(awsEnv, "host1", ec2.WithOS(componentsOs.UbuntuDefault))
		if err != nil {
			return err
		}
		host1.Export(ctx, &env.Host1.HostOutput)

		host2, err := ec2.NewVM(awsEnv, "host2", ec2.WithOS(componentsOs.UbuntuDefault))
		if err != nil {
			return err
		}
		host2.Export(ctx, &env.Host2.HostOutput)

		agent1, err := agent.NewHostAgent(&awsEnv, host1, agentparams.WithAgentConfig(agentConfig1))
		if err != nil {
			return err
		}
		agent1.Export(ctx, &env.Agent1.HostAgentOutput)

		agent2, err := agent.NewHostAgent(&awsEnv, host2, agentparams.WithAgentConfig(agentConfig2))
		if err != nil {
			return err
		}
		agent2.Export(ctx, &env.Agent2.HostAgentOutput)

		configuration := datadog.NewConfiguration()

		apiClient := datadog.NewAPIClient(configuration)
		api := datadogV1.NewMetricsApi(apiClient)
		env.API = api

		return nil
	}
}

type multiVMSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestMultiVMSuite(t *testing.T) {
	e2e.Run(t, &multiVMSuite{}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil))
}

func (v *multiVMSuite) TestHAFailover() {
	initialActiveAgent := ""
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent state switched from unknown to active in agent.log")

		output1, err := v.Env().Host1.Execute("cat /var/log/datadog/agent.log")
		require.NoError(c, err)
		assert.Contains(c, output1, "Add HA Agent RCListener")

		output2, err := v.Env().Host2.Execute("cat /var/log/datadog/agent.log")
		require.NoError(c, err)
		assert.Contains(c, output2, "Add HA Agent RCListener")

		require.True(c, strings.Contains(output1, "agent state switched from unknown to active") || strings.Contains(output2, "agent state switched from unknown to active"), "Expected one of the agents to switch from unknown to active")

		if strings.Contains(output1, "agent state switched from unknown to active") {
			initialActiveAgent = "agent1"
		} else {
			initialActiveAgent = "agent2"
		}
	}, 5*time.Minute, 30*time.Second)

	if initialActiveAgent == "" {
		v.T().Fatal("No agent switched to active")
	}

	if initialActiveAgent == "agent1" {
		v.Env().Host1.Execute("sudo systemctl stop datadog-agent")
	} else {
		v.Env().Host2.Execute("sudo systemctl stop datadog-agent")
	}

	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert active agent election has been made")

		if initialActiveAgent == "agent1" {
			output2, err := v.Env().Host2.Execute("cat /var/log/datadog/agent.log")
			require.NoError(c, err)
			assert.Contains(c, output2, "agent state switched from standby to active")
		} else {
			output1, err := v.Env().Host1.Execute("cat /var/log/datadog/agent.log")
			require.NoError(c, err)
			assert.Contains(c, output1, "agent state switched from standby to active")
		}
	}, 5*time.Minute, 30*time.Second)

	if initialActiveAgent == "agent1" {
		v.Env().Host1.Execute("sudo systemctl start datadog-agent")
	} else {
		v.Env().Host2.Execute("sudo systemctl start datadog-agent")
	}

	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert active agent stays the same after failover")

		if initialActiveAgent == "agent1" {
			output1, err := v.Env().Host1.Execute("cat /var/log/datadog/agent.log")
			require.NoError(c, err)
			assert.Contains(c, output1, "agent state switched from unknown to active")
		} else {
			output2, err := v.Env().Host2.Execute("cat /var/log/datadog/agent.log")
			require.NoError(c, err)
			assert.Contains(c, output2, "agent state switched from unknown to active")
		}
	}, 5*time.Minute, 30*time.Second)
}
