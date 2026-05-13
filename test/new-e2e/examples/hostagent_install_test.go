// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

// This file demonstrates the recommended pattern for host-based E2E tests
// using the non-Pulumi installer approach:
//
//  1. The Pulumi provisioner creates only infrastructure (VM + fakeintake).
//  2. hostagent.Install installs and configures the agent via SSH in SetupSuite,
//     decoupled from Pulumi's lifecycle.
//  3. agent.Configure reconfigures the agent mid-suite without re-running
//     Pulumi, replacing the old UpdateEnv pattern for agent-only changes.
//
// Compared to the old approach (passing ec2.WithAgentOptions to the provisioner),
// this pattern makes agent install and config changes faster and independent of
// Pulumi state, and it works in the same way on any remote host regardless of
// how the infrastructure was provisioned.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type hostAgentInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestHostAgentInstall(t *testing.T) {
	// The provisioner provisions a VM and a fakeintake, but no agent.
	// Agent installation is handled in SetupSuite by hostagent.Install.
	e2e.Run(t, &hostAgentInstallSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(awshost.WithRunOptions(scenec2.WithoutAgent())),
	))
}

func (s *hostAgentInstallSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// Install the agent on the provisioned VM via SSH.
	// hostagent.Install automatically connects the agent to s.Env().FakeIntake
	// and populates s.Env().Agent with a ready-to-use agent client.
	hostagent.Install(s.T(), s.Env(),
		agentparams.WithAgentConfig("log_level: info"),
	)
}

// TestFakeIntakeReceivesMetrics verifies the SSH-installed agent sends metrics
// to the fakeintake, the same way it would with a Pulumi-installed agent.
func (s *hostAgentInstallSuite) TestFakeIntakeReceivesMetrics() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.load.1")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

// TestFakeIntakeReceivesSystemUptime shows filtering metrics by value.
func (s *hostAgentInstallSuite) TestFakeIntakeReceivesSystemUptime() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.uptime' with value > 0 yet")
	}, 5*time.Minute, 10*time.Second)
}

// TestReconfigure shows agent.Configure as a lightweight alternative to
// UpdateEnv: it rewrites config files and restarts the agent via SSH
// without touching the Pulumi stack, making config-only changes faster.
func (s *hostAgentInstallSuite) TestReconfigure() {
	s.Env().Agent.Configure(s.T(),
		agentparams.WithAgentConfig("log_level: debug"),
	)
	assert.Contains(s.T(), s.Env().Agent.Client.Config(), "log_level: debug")
}
