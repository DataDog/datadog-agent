// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	componentsOs "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed fixtures/snmp_conf.yaml
var snmpConfig string

//go:embed fixtures/cisco_aci_conf.yaml
var ciscoAciConfig string

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
config_id: ci-e2e-ha-failover
log_level: debug
`

		agentConfig2 := `
hostname: test-e2e-agent2
ha_agent:
    enabled: true
config_id: ci-e2e-ha-failover
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

		agent1, err := agent.NewHostAgent(&awsEnv, host1, agentparams.WithAgentConfig(agentConfig1), agentparams.WithIntegration("snmp.d", snmpConfig), agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig))
		if err != nil {
			return err
		}
		agent1.Export(ctx, &env.Agent1.HostAgentOutput)

		agent2, err := agent.NewHostAgent(&awsEnv, host2, agentparams.WithAgentConfig(agentConfig2), agentparams.WithIntegration("snmp.d", snmpConfig), agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig))
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

type haAgentMetadata struct {
	State string `json:"state"`
}

type metadataPayload struct {
	Metadata haAgentMetadata `json:"ha_agent_metadata"`
}

func (v *testHAAgentFailoverSuite) assertHAState(c *assert.CollectT, agent *components.RemoteHostAgent, expectedState string) {
	output := agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "ha-agent"}))

	var payload metadataPayload
	err := json.Unmarshal([]byte(output), &payload)
	require.NoError(c, err)

	state := payload.Metadata.State
	require.Equal(c, expectedState, state, "Expected agent to be %s", expectedState)
}

func (v *testHAAgentFailoverSuite) assertCheckIsRunning(c *assert.CollectT, agent *components.RemoteHostAgent, checkName string) {
	output := agent.Client.Status(agentclient.WithArgs([]string{"collector"}))

	require.Contains(c, output.Content, checkName, fmt.Sprintf("Expected %s to be running", checkName))
}

func (v *testHAAgentFailoverSuite) assertCheckIsNotRunning(c *assert.CollectT, agent *components.RemoteHostAgent, checkName string) {
	output := agent.Client.Status(agentclient.WithArgs([]string{"collector"}))

	require.NotContains(c, output.Content, checkName, fmt.Sprintf("Expected %s to be not running", checkName))
}

func (v *testHAAgentFailoverSuite) TestHAFailover() {
	v.Env().Host2.Execute("sudo systemctl stop datadog-agent")

	// Wait for the agent1 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 state is active")
		v.assertHAState(c, v.Env().Agent1, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent1 is running HA and non-HA checks
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 is running HA and non-HA checks")
		v.assertCheckIsRunning(c, v.Env().Agent1, "snmp")
		v.assertCheckIsRunning(c, v.Env().Agent1, "cisco_aci")
		v.assertCheckIsRunning(c, v.Env().Agent1, "cpu")
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host2.Execute("sudo systemctl start datadog-agent")

	// Wait for the agent2 to be standby
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is standby")
		v.assertHAState(c, v.Env().Agent2, "standby")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is not running the HA checks but is running the non-HA checks
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is not running the HA checks but is running the non-HA checks")
		v.assertCheckIsNotRunning(c, v.Env().Agent2, "snmp")
		v.assertCheckIsNotRunning(c, v.Env().Agent2, "cisco_aci")
		v.assertCheckIsRunning(c, v.Env().Agent2, "cpu")
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host1.Execute("sudo systemctl stop datadog-agent")

	// Wait for the agent2 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is active")
		v.assertHAState(c, v.Env().Agent2, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is running HA and non-HA checks
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is running HA and non-HA checks")
		v.assertCheckIsRunning(c, v.Env().Agent2, "snmp")
		v.assertCheckIsRunning(c, v.Env().Agent2, "cisco_aci")
		v.assertCheckIsRunning(c, v.Env().Agent2, "cpu")
	}, 5*time.Minute, 10*time.Second)

	v.Env().Host1.Execute("sudo systemctl start datadog-agent")

	// Wait for the agent1 to be standby
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 state is standby")
		v.assertHAState(c, v.Env().Agent1, "standby")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent1 is not running HA checks but is running the non-HA checks
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent1 is not running HA checks but is running the non-HA checks")
		v.assertCheckIsNotRunning(c, v.Env().Agent1, "snmp")
		v.assertCheckIsNotRunning(c, v.Env().Agent1, "cisco_aci")
		v.assertCheckIsRunning(c, v.Env().Agent1, "cpu")
	}, 5*time.Minute, 10*time.Second)

	// Wait for the agent2 to be active
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 state is active")
		v.assertHAState(c, v.Env().Agent2, "active")
	}, 5*time.Minute, 30*time.Second)

	// Check that agent2 is running HA and non-HA checks
	v.EventuallyWithT(func(c *assert.CollectT) {
		v.T().Log("try assert agent2 is running HA and non-HA checks")
		v.assertCheckIsRunning(c, v.Env().Agent2, "snmp")
		v.assertCheckIsRunning(c, v.Env().Agent2, "cisco_aci")
		v.assertCheckIsRunning(c, v.Env().Agent2, "cpu")
	}, 5*time.Minute, 10*time.Second)
}
