// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	_ "embed"
	"encoding/json"
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

	haagent "github.com/DataDog/datadog-agent/comp/metadata/haagent/impl"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed fixtures/snmp_conf.yaml
var snmpConfig string

type multiVMEnv struct {
	Host1  *components.RemoteHost
	Host2  *components.RemoteHost
	Agent1 *components.RemoteHostAgent
	Agent2 *components.RemoteHostAgent
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

		agent1, err := agent.NewHostAgent(&awsEnv, host1, agentparams.WithAgentConfig(agentConfig1), agentparams.WithIntegration("snmp.d", snmpConfig))
		if err != nil {
			return err
		}
		agent1.Export(ctx, &env.Agent1.HostAgentOutput)

		agent2, err := agent.NewHostAgent(&awsEnv, host2, agentparams.WithAgentConfig(agentConfig2), agentparams.WithIntegration("snmp.d", snmpConfig))
		if err != nil {
			return err
		}
		agent2.Export(ctx, &env.Agent2.HostAgentOutput)

		return nil
	}
}

type testHAAgentFailoverSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestHAAgentFailoverSuite(t *testing.T) {
	e2e.Run(t, &testHAAgentFailoverSuite{}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil))
}

func (v *testHAAgentFailoverSuite) assertHAState(c *assert.CollectT, host *components.RemoteHost, expectedState string) {
	output, err := host.Execute("sudo datadog-agent diagnose show-metadata ha-agent --json")
	require.NoError(c, err)

	var payload haagent.Payload
	err = json.Unmarshal([]byte(output), &payload)
	require.NoError(c, err)

	state, ok := payload.Metadata["state"]
	require.True(c, ok, "Expected state to be present in metadata")
	require.Equal(c, expectedState, state, "Expected agent to be %s", expectedState)
}

func (v *testHAAgentFailoverSuite) assertSNMPCheckIsRunning(c *assert.CollectT, host *components.RemoteHost) {
	output, err := host.Execute("sudo datadog-agent status collector")
	require.NoError(c, err)

	require.Contains(c, output, "snmp", "Expected snmp to be running")
}

func (v *testHAAgentFailoverSuite) assertSNMPCheckIsNotRunning(c *assert.CollectT, host *components.RemoteHost) {
	output, err := host.Execute("sudo datadog-agent status collector")
	require.NoError(c, err)

	require.NotContains(c, output, "snmp", "Expected snmp to be running")
}

func (v *testHAAgentFailoverSuite) TestHAFailover() {
	v.Env().Host2.Execute("sudo systemctl stop datadog-agent")

	// Wait for the agent1 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 state is active")
		v.assertHAState(c, v.Env().Host1, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent1 is running the SNMP check
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 is running the SNMP check")
		v.assertSNMPCheckIsRunning(c, v.Env().Host1)
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host2.Execute("sudo systemctl start datadog-agent")

	// Wait for the agent2 to be standby
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is standby")
		v.assertHAState(c, v.Env().Host2, "standby")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is not running the SNMP check
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is not running the SNMP check")
		v.assertSNMPCheckIsNotRunning(c, v.Env().Host2)
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host1.Execute("sudo systemctl stop datadog-agent")

	// Wait for the agent2 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is active")
		v.assertHAState(c, v.Env().Host2, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is running the SNMP check
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is running the SNMP check")
		v.assertSNMPCheckIsRunning(c, v.Env().Host2)
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host1.Execute("sudo systemctl start datadog-agent")

	// Wait for the agent1 to be standby
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 state is standby")
		v.assertHAState(c, v.Env().Host1, "standby")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent1 is not running the SNMP check
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 is not running the SNMP check")
		v.assertSNMPCheckIsNotRunning(c, v.Env().Host1)
	}, 5*time.Minute, 10*time.Second)

	// Wait for the agent2 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is active")
		v.assertHAState(c, v.Env().Host2, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is running the SNMP check
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is running the SNMP check")
		v.assertSNMPCheckIsRunning(c, v.Env().Host2)
	}, 5*time.Minute, 10*time.Second)
}
