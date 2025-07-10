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
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
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
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "info")

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
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")

	// Promote config experiment
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")
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
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StartConfigExperiment(consts.AgentPackage, config)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))

	// Wait for watchdog to restart the services with the stable config
	s.WaitForServicesWithBackoff("Running", backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10),
		consts.ServiceName,
		"datadogagent",
	)
	s.AssertSuccessfulConfigStopExperiment()

	// Config should be reverted to the stable config
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "info")

	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
		s.AssertSuccessfulConfigStopExperiment()
	})
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

	// Start config experiment (restarts the services)
	s.mustStartConfigExperiment(config)

	// Wait for all services to be running
	// 30*10 -> 300 seconds (5 minutes)
	expectedServices := []string{
		"datadogagent",
		"datadog-system-probe",
		"datadog-security-agent",
		"datadog-process-agent",
		"ddnpm",
		"ddprocmon",
	}
	b := backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10)
	err = s.WaitForServicesWithBackoff("Running", b, expectedServices...)
	s.Require().NoError(err, "Failed waiting for services to start")

	// Promote config experiment (restarts the services)
	s.mustPromoteConfigExperiment(config)

	// Wait for all services to be running
	// 30*10 -> 300 seconds (5 minutes)
	err = s.WaitForServicesWithBackoff("Running", b, expectedServices...)
	s.Require().NoError(err, "Failed waiting for services to start")
}

// TestRevertsConfigExperimentWhenServiceDies tests that the watchdog will revert
// to stable config when the service dies.
func (s *testAgentConfigSuite) TestRevertsConfigExperimentWhenServiceDies() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

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

	// Start config experiment (restarts the services)
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")

	// Stop the agent service to trigger watchdog rollback
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	s.AssertSuccessfulConfigStopExperiment()

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "info")

	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.AssertSuccessfulConfigStopExperiment()
	})
}

// TestRevertsConfigExperimentWhenTimeout tests that the watchdog will revert
// to stable config when the timeout expires.
func (s *testAgentConfigSuite) TestRevertsConfigExperimentWhenTimeout() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	// lower timeout to 2 minutes
	s.setWatchdogTimeout(2)

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
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")

	// wait for the timeout
	s.WaitForDaemonToStop(func() {}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))

	// Assert
	s.AssertSuccessfulConfigStopExperiment()

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "info")

	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.AssertSuccessfulConfigStopExperiment()
	})
}

// TestManagedConfigActiveAfterUpgrade tests that the Agent's config is preserved after a package update.
//
// Partial regression test for WINA-1556, making sure that installation does not
// modify the managed config or its permissions, preventing the Agent from accessing it.
func (s *testAgentConfigSuite) TestManagedConfigActiveAfterUpgrade() {
	// Arrange - Start with previous version and custom config
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "info")

	// Set up a custom configuration
	config := installerwindows.ConfigExperiment{
		ID: "pre-upgrade-config",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_level": "debug"}`),
			},
		},
	}

	// Start and promote the config experiment to make it the stable config
	s.mustStartConfigExperiment(config)
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")

	// Act - Perform a package upgrade to current version
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert - Verify that the configuration is preserved after the upgrade
	// The promoted config should still be active after upgrade
	s.AssertSuccessfulConfigPromoteExperiment(config.ID)

	// Verify the runtime config values are still preserved after upgrade
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_level", "debug")
}

func (s *testAgentConfigSuite) mustStartConfigExperiment(config installerwindows.ConfigExperiment) {
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StartConfigExperiment(consts.AgentPackage, config)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))

	s.AssertSuccessfulConfigStartExperiment(config.ID)
}

func (s *testAgentConfigSuite) mustPromoteConfigExperiment(config installerwindows.ConfigExperiment) {
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().PromoteConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))

	s.AssertSuccessfulConfigPromoteExperiment(config.ID)
}
