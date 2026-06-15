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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	componentsOs "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// multiVMEnvProvisioner returns a provisioner for two VMs with HA agents.
// configID is generated once outside the Pulumi closure so both closures
// (Pulumi + PostProvision) share the same value.
func multiVMEnvProvisioner() provisioners.TypedProvisioner[multiVMEnv] {
	// Generate config_id outside the closure so it's stable across retries.
	randomConfigID := fmt.Sprintf("ci-e2e-ha-failover-%d", rand.Intn(10))

	agentConfig1 := fmt.Sprintf(`
hostname: test-e2e-agent1
ha_agent:
    enabled: true
config_id: %s
log_level: debug
`, randomConfigID)

	agentConfig2 := fmt.Sprintf(`
hostname: test-e2e-agent2
ha_agent:
    enabled: true
config_id: %s
log_level: debug
`, randomConfigID)

	pulumiProv := provisioners.NewTypedPulumiProvisioner("aws-ha-multivm", func(ctx *pulumi.Context, env *multiVMEnv) error {
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

		// Agent installation moved to PostProvision.
		env.Agent1 = nil
		env.Agent2 = nil
		return nil
	}, nil)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *multiVMEnv) {
		ctx := installers.FromT(t)
		env.Agent1 = hostagent.InstallOnHost(ctx, env.Host1, nil,
			agentparams.WithAgentConfig(agentConfig1),
			agentparams.WithIntegration("snmp.d", snmpConfig),
			agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig),
		)
		env.Agent2 = hostagent.InstallOnHost(ctx, env.Host2, nil,
			agentparams.WithAgentConfig(agentConfig2),
			agentparams.WithIntegration("snmp.d", snmpConfig),
			agentparams.WithIntegration("cisco_aci.d", ciscoAciConfig),
		)
	})
}

type testHAAgentFailoverSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestHAAgentFailoverSuite(t *testing.T) {
	e2e.Run(t, &testHAAgentFailoverSuite{}, e2e.WithProvisioner(multiVMEnvProvisioner()))
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
