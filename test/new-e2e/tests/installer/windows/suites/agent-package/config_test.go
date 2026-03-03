// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
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
	configRoot := windowsagent.DefaultConfigRoot
	configBackupRoot := windowsagent.DefaultConfigRoot + "-exp"

	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true)

	// collect permissions snapshot before config experiment
	perms, err := windowscommon.GetSecurityInfoForPath(s.Env().RemoteHost, configRoot)
	s.Require().NoError(err, "should get security info for config root")
	configSDDLBeforeExperiment := perms.SDDL

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false).
		HasDDAgentUserFileAccess()
	perms, err = windowscommon.GetSecurityInfoForPath(s.Env().RemoteHost, configBackupRoot)
	s.Require().NoError(err, "should get security info for config backup root")
	s.Require().Equal(configSDDLBeforeExperiment, perms.SDDL, "backup dir permissions should be the same as the config dir permissions")

	// Promote config experiment
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false).
		HasDDAgentUserFileAccess().
		NoDirExists(configBackupRoot) // backup dir should be deleted

	// assert that the config dir permissions have not changed
	perms, err = windowscommon.GetSecurityInfoForPath(s.Env().RemoteHost, configRoot)
	s.Require().NoError(err, "should get security info for config root")
	s.Require().Equal(configSDDLBeforeExperiment, perms.SDDL, "config dir permissions should not have changed")
}

// TestConfigUpgradeFailure tests that the Agent's config can be rolled back
// through the experiment (start/promote) workflow.
func (s *testAgentConfigSuite) TestConfigUpgradeFailure() {
	configRoot := windowsagent.DefaultConfigRoot
	configBackupRoot := windowsagent.DefaultConfigRoot + "-exp"

	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_level", "debug")

	// collect permissions snapshot before config experiment
	perms, err := windowscommon.GetSecurityInfoForPath(s.Env().RemoteHost, configRoot)
	s.Require().NoError(err, "should get security info for config root")
	configSDDLBeforeExperiment := perms.SDDL

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path: "/datadog.yaml",
				// TODO: This used to trigger an "unknown secret" error that would
				//       cause the Agent to fail to start. Now it's "unknown log level"
				//       and with other options the Agent starts just fine, so keep at
				//       using log_level for now.
				Contents: json.RawMessage(`{"log_level": "ENC[hi]"}`), // Invalid config
			},
		},
	}

	// Start config experiment, block until services stop
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StartConfigExperiment(consts.AgentPackage, config)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	// Wait for watchdog to restart the services with the stable config
	s.WaitForServicesWithBackoff("Running", []string{consts.ServiceName, "datadogagent"},
		backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))
	s.AssertSuccessfulConfigStopExperiment()

	// Config should be reverted to the stable config
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_level", "debug").
		HasDDAgentUserFileAccess().
		NoDirExists(configBackupRoot) // backup dir should be deleted

	// backend will send stop experiment now
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
		s.AssertSuccessfulConfigStopExperiment()
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	// assert that the config dir permissions have not changed
	perms, err = windowscommon.GetSecurityInfoForPath(s.Env().RemoteHost, configRoot)
	s.Require().NoError(err, "should get security info for config root")
	s.Require().Equal(configSDDLBeforeExperiment, perms.SDDL, "config dir permissions should not have changed")
}

// TestConfigUpgradeNewAgents tests that config experiments can enable security agent and system probe
// on new agent installations.
func (s *testAgentConfigSuite) TestConfigUpgradeNewAgents() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")

	// Assert that the non-default services are not running
	err := s.WaitForServicesWithBackoff("Stopped", []string{
		"datadog-system-probe",
		"datadog-security-agent",
		"ddnpm",
		"ddprocmon",
	}, backoff.WithBackOff(&backoff.StopBackOff{}))
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
	retryOpts := []backoff.RetryOption{backoff.WithBackOff(backoff.NewConstantBackOff(30 * time.Second)), backoff.WithMaxTries(10)}
	err = s.WaitForServicesWithBackoff("Running", expectedServices, retryOpts...)
	s.Require().NoError(err, "Failed waiting for services to start")

	// Promote config experiment (restarts the services)
	s.mustPromoteConfigExperiment(config)

	// Wait for all services to be running
	// 30*10 -> 300 seconds (5 minutes)
	err = s.WaitForServicesWithBackoff("Running", expectedServices, retryOpts...)
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
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment (restarts the services)
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)

	// Stop the agent service to trigger watchdog rollback
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	s.AssertSuccessfulConfigStopExperiment()

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true).
		HasDDAgentUserFileAccess()

	// backend will send stop experiment now
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.AssertSuccessfulConfigStopExperiment()
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))
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
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)

	// wait for the timeout
	s.WaitForDaemonToStop(func() {}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	// Assert
	s.AssertSuccessfulConfigStopExperiment()

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true).
		HasDDAgentUserFileAccess()

	// backend will send stop experiment now
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.AssertSuccessfulConfigStopExperiment()
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))
}

// TestManagedConfigActiveAfterUpgrade tests that the Agent's config is preserved after a package update.
//
// Partial regression test for WINA-1556, making sure that installation does not
// modify the managed config or its permissions, preventing the Agent from accessing it.
func (s *testAgentConfigSuite) TestManagedConfigActiveAfterUpgrade() {
	s.T().Skip("Skipping test during migration to new config experiment")
	// Arrange - Start with previous version and custom config
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_to_console", true)

	// Set up a custom configuration
	config := installerwindows.ConfigExperiment{
		ID: "pre-upgrade-config",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start and promote the config experiment to make it the stable config
	s.mustStartConfigExperiment(config)
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig().
		WithValueEqual("log_to_console", false)

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
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)
}

// TestConfigAltDir tests that the Agent's config can be updated
// when using an alternate config and install path.
func (s *testAgentConfigSuite) TestConfigAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.installCurrentAgentVersion(
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true)

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)

	// Promote config experiment
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasDDAgentUserFileAccess().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`)
}

func (s *testAgentConfigSuite) TestConfigCustomUser() {
	// Arrange
	agentUser := "customuser"
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installCurrentAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
	)
	// sanity check that the agent is running as the custom user
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)

	// Assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true)

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)

	// Promote config experiment
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("all").
		WithValueEqual("log_to_console", false).
		HasDDAgentUserFileAccess(agentUser).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)
}

func (s *testAgentConfigSuite) TestConfigCustomUserAndAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	agentUser := "customuser"
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installCurrentAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Assert that setup was successful
	s.AssertSuccessfulConfigPromoteExperiment("empty")
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", true)

	// Act
	config := installerwindows.ConfigExperiment{
		ID: "config-1",
		Files: []installerwindows.ConfigExperimentFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"log_to_console": false}`),
			},
		},
	}

	// Start config experiment
	s.mustStartConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("--all").
		WithValueEqual("log_to_console", false)

	// Promote config experiment
	s.mustPromoteConfigExperiment(config)

	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().RuntimeConfig("all").
		WithValueEqual("log_to_console", false).
		HasDDAgentUserFileAccess(agentUser).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`)
}

func (s *testAgentConfigSuite) mustStartConfigExperiment(config installerwindows.ConfigExperiment) {
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StartConfigExperiment(consts.AgentPackage, config)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	s.AssertSuccessfulConfigStartExperiment(config.ID)
}

func (s *testAgentConfigSuite) mustPromoteConfigExperiment(config installerwindows.ConfigExperiment) {
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().PromoteConfigExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)), backoff.WithMaxTries(10))

	s.AssertSuccessfulConfigPromoteExperiment(config.ID)
}
