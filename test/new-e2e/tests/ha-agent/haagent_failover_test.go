// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	componentsOs "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

//go:embed fixtures/snmp_conf.yaml
var snmpConfig string

//go:embed fixtures/cisco_aci_conf.yaml
var ciscoAciConfig string

type multiVMEnv struct {
	Host1      *components.RemoteHost
	Host2      *components.RemoteHost
	Agent1     *components.RemoteHostAgent
	Agent2     *components.RemoteHostAgent
	FakeIntake *components.FakeIntake
}

const (
	agent1Hostname = "test-e2e-agent1"
	agent2Hostname = "test-e2e-agent2"
)

func multiVMEnvProvisioner(configID string) provisioners.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {

		// language=yaml
		agentConfig1 := fmt.Sprintf(`
hostname: %s
ha_agent:
    enabled: true
config_id: %s
log_level: debug
`, agent1Hostname, configID)

		agentConfig2 := fmt.Sprintf(`
hostname: %s
ha_agent:
    enabled: true
config_id: %s
log_level: debug
`, agent2Hostname, configID)

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

		// Share a single fakeintake between both agents so RC can be driven from the
		// test instead of relying on the real Datadog backend to elect a leader.
		fakeIntake, err := fakeintakescenario.NewECSFargateInstance(awsEnv, "ha-failover")
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		agent1, err := agent.NewHostAgent(&awsEnv, host1, agentparams.WithFakeintake(fakeIntake), agentparams.WithAgentConfig(agentConfig1), agentparams.WithIntegration("snmp.d", snmpConfig), agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig))
		if err != nil {
			return err
		}
		agent1.Export(ctx, &env.Agent1.HostAgentOutput)

		agent2, err := agent.NewHostAgent(&awsEnv, host2, agentparams.WithFakeintake(fakeIntake), agentparams.WithAgentConfig(agentConfig2), agentparams.WithIntegration("snmp.d", snmpConfig), agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig))
		if err != nil {
			return err
		}
		agent2.Export(ctx, &env.Agent2.HostAgentOutput)

		return nil
	}
}

type testHAAgentFailoverSuite struct {
	e2e.BaseSuite[multiVMEnv]

	configID string
}

func TestHAAgentFailoverSuite(t *testing.T) {
	// Generate a random config_id (0-9) to avoid conflicts when tests run in parallel
	configID := fmt.Sprintf("ci-e2e-ha-failover-%d", rand.Intn(10))

	e2e.Run(t, &testHAAgentFailoverSuite{configID: configID}, e2e.WithPulumiProvisioner(multiVMEnvProvisioner(configID), nil))
}

// setActiveAgent pushes an HA_AGENT Remote Config payload to fakeintake, electing
// activeHostname as the active agent. This replaces the automatic failover decision
// that the real Datadog backend would otherwise make based on agent liveness.
func (v *testHAAgentFailoverSuite) setActiveAgent(activeHostname string) {
	fakeClient := v.Env().FakeIntake.Client()

	payload := fmt.Sprintf(`{"config_id":%q,"active_agent":%q}`, v.configID, activeHostname)
	err := fakeClient.RCAddConfig("", state.ProductHaAgent, "ha-failover", "leader", []byte(payload))
	require.NoError(v.T(), err)
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

	// Elect agent1 as active via Remote Config, simulating the backend's failover
	// decision now that agent2 is down.
	v.setActiveAgent(agent1Hostname)

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

	// Restart through the Agent client so its readiness state is reset. The next
	// client command will then wait for the Agent API to become available.
	require.NoError(v.T(), v.Env().Agent2.Client.Restart())

	// agent2 comes back up, polls Remote Config, and picks up the existing
	// "agent1 active" config, so it settles into standby on its own.

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

	// Elect agent2 as active via Remote Config, simulating the backend's failover
	// decision now that agent1 is down.
	v.setActiveAgent(agent2Hostname)

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

	// Restart through the Agent client so its readiness state is reset. Without
	// this, the cached ready state makes the next command race Agent startup.
	require.NoError(v.T(), v.Env().Agent1.Client.Restart())

	// agent1 comes back up, polls Remote Config, and picks up the existing
	// "agent2 active" config, so it settles into standby on its own.

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
