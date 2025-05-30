// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testAgentConfigSuite struct {
	testAgentUpgradeSuite
}

// TestAgentConfig tests the usage of the Datadog installer to manage Agent configuration.
func TestAgentConfig(t *testing.T) {
	e2e.Run(t, &testAgentConfigSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestConfigUpgradeSuccessful tests that the Agent's config can be upgraded
// through the experiment (start/promote) workflow.
func (s *testAgentConfigSuite) TestConfigUpgradeSuccessful() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_level": "debug"}`),
			},
		},
	}

	// Start config experiment
	_, err := s.Installer().InstallConfigExperiment(consts.AgentPackage, config)
	s.Require().NoError(err)
	s.AssertSuccessfulConfigStartExperiment(config.ID)

	// Promote config experiment
	_, err = s.Installer().PromoteConfigExperiment(consts.AgentPackage)
	s.Require().NoError(err)
	s.AssertSuccessfulConfigPromoteExperiment(config.ID)
}

// TestConfigUpgradeFailure tests that the Agent's config can be rolled back
// through the experiment (start/promote) workflow.
func (s *testAgentConfigSuite) TestConfigUpgradeFailure() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_level": "ENC[hi]"}`), // Invalid config
			},
		},
	}

	// Start config experiment, block until services stop
	s.waitForDaemonToStop(func() {
		_, err := s.Installer().InstallConfigExperiment(consts.AgentPackage, config)
		s.Require().NoError(err)
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))

	// Assert services failed to start with invalid config
	s.Require().Host(s.Env().RemoteHost).
		HasAService(consts.ServiceName).
		WithStatus("Stopped")

	// Stop config experiment
	_, err := s.Installer().RemoveConfigExperiment(consts.AgentPackage)
	s.Require().NoError(err)
	s.AssertSuccessfulConfigStopExperiment()

	// Wait for services to be running again
	s.WaitForServicesWithBackoff("Running", backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10),
		consts.ServiceName,
		"datadogagent",
	)
}

// TestConfigUpgradeNewAgents tests that config experiments can enable security agent and system probe
// on new agent installations.
func (s *testAgentConfigSuite) TestConfigUpgradeNewAgents() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Assert that the non-default services are not running
	err := s.WaitForServicesWithBackoff("Stopped", &backoff.StopBackOff{},
		"datadog-system-probe",
		"datadog-security-agent",
		"ddnpm",
		"ddprocmon",
	)
	s.Require().NoError(err, "non-default services should not be running")

	// Act
	// Set config values that will cause the Agent to start the non-default services
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"process_config": {"process_collection": {"enabled": true}}}`),
			},
			{
				Path:     "/security-agent.yaml",
				Contents: json.RawMessage(`{"runtime_security_config": {"enabled": true}}`),
			},
			{
				Path:     "/system-probe.yaml",
				Contents: json.RawMessage(`{"runtime_security_config": {"enabled": true}, "network_config": {"enabled": true}}`),
			},
		},
	}

	// Start config experiment
	_, err = s.Installer().InstallConfigExperiment(consts.AgentPackage, config)
	s.Require().NoError(err)
	s.AssertSuccessfulConfigStartExperiment(config.ID)

	// Promote config experiment
	_, err = s.Installer().PromoteConfigExperiment(consts.AgentPackage)
	s.Require().NoError(err)
	s.AssertSuccessfulConfigPromoteExperiment(config.ID)

	// Wait for all services to be running
	// 30*10 -> 300 seconds (5 minutes)
	b := backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10)
	err = s.WaitForServicesWithBackoff("Running", b,
		"datadogagent",
		"datadog-system-probe",
		"datadog-security-agent",
		"datadog-process-agent",
		"ddnpm",
		"ddprocmon",
	)
	s.Require().NoError(err, "Failed waiting for services to start")
}
