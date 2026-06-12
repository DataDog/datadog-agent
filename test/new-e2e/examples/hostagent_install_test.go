// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

// This file demonstrates two patterns for host-based E2E tests using the
// installer approach (agent installed via SSH, not Pulumi):
//
// Pattern A – Canonical (recommended): pass agent options to the provisioner
// via WithRunOptions(ec2.WithAgentOptions(...)). The provisioner installs the
// agent via PostProvision automatically. Tests look identical to the old Pulumi
// approach; only the install mechanism changes under the hood.
//
// Pattern B – Explicit: suppress Pulumi agent install with WithoutAgent and
// call hostagent.Install yourself in SetupSuite. Use this for bespoke
// environments where you need fine control over installation order, or when
// the provisioner wrapper does not yet support your scenario.
//
// In both patterns, agent.Configure reconfigures the running agent mid-suite
// via SSH without re-running Pulumi, replacing the old UpdateEnv pattern for
// agent-only changes.

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

// ── Pattern A: canonical, no SetupSuite override needed ──────────────────────

type hostAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestHostAgent demonstrates the canonical pattern: pass agent options to the
// provisioner. The provisioner installs the agent via SSH automatically
// (PostProvision), the test body is identical to the old Pulumi-based approach.
func TestHostAgent(t *testing.T) {
	e2e.Run(t, &hostAgentSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(awshost.WithRunOptions(
			scenec2.WithAgentOptions(
				agentparams.WithAgentConfig("log_level: info"),
			),
		)),
	))
}

func (s *hostAgentSuite) TestFakeIntakeReceivesMetrics() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.load.1")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}

func (s *hostAgentSuite) TestFakeIntakeReceivesSystemUptime() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.uptime", client.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.uptime' with value > 0 yet")
	}, 5*time.Minute, 10*time.Second)
}

// TestReconfigure shows agent.Configure as a lightweight alternative to
// UpdateEnv: it rewrites config files and restarts the agent via SSH without
// touching the Pulumi stack, making config-only changes fast.
func (s *hostAgentSuite) TestReconfigure() {
	s.Env().Agent.Configure(s.T(),
		agentparams.WithAgentConfig("log_level: debug"),
	)
	assert.Contains(s.T(), s.Env().Agent.Client.Config(), "log_level: debug")
}

// ── Pattern B: explicit, for bespoke install scenarios ───────────────────────

type hostAgentExplicitInstallSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestHostAgentExplicitInstall demonstrates the explicit pattern: suppress
// Pulumi agent install with WithoutAgent, then call hostagent.Install yourself
// in SetupSuite. Useful when you need control over the installation order or
// when installing into a custom environment type.
func TestHostAgentExplicitInstall(t *testing.T) {
	e2e.Run(t, &hostAgentExplicitInstallSuite{}, e2e.WithProvisioner(
		// WithoutAgent tells the provisioner to provision infra only (VM + fakeintake),
		// leaving agent installation to SetupSuite below.
		awshost.Provisioner(awshost.WithRunOptions(scenec2.WithoutAgent())),
	))
}

func (s *hostAgentExplicitInstallSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	// Install the agent on the provisioned VM via SSH. hostagent.Install
	// automatically connects the agent to s.Env().FakeIntake and populates
	// s.Env().Agent with a ready-to-use agent client.
	hostagent.Install(s.T(), s.Env(),
		agentparams.WithAgentConfig("log_level: info"),
	)
}

func (s *hostAgentExplicitInstallSuite) TestFakeIntakeReceivesMetrics() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.load.1")
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'system.load.1' metrics yet")
	}, 5*time.Minute, 10*time.Second)
}
