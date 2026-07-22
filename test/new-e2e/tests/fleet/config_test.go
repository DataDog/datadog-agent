// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fleet contains tests for fleet
package fleet

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/ddot"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

type configSuite struct {
	suite.FleetSuite
}

func newConfigSuite() e2e.Suite[environments.Host] {
	return &configSuite{}
}

func TestFleetConfig(t *testing.T) {
	suite.Run(t, newConfigSuite, suite.Platforms())
}

func (s *configSuite) TestConfig() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
	}, nil)
	require.NoError(s.T(), err)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])
}

func (s *configSuite) TestMultipleConfigs() {
	s.Agent.MustInstall(agent.WithRemoteUpdates())
	defer s.Agent.MustUninstall()

	for i := 0; i < 3; i++ {
		err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
			DeploymentID: fmt.Sprintf("123-%d", i),
			FileOperations: []backend.FileOperation{
				{
					FileOperationType: backend.FileOperationMergePatch,
					FilePath:          "/datadog.yaml",
					Patch:             []byte(fmt.Sprintf(`{"extra_tags": ["debug:step-%d"]}`, i)),
				},
			},
		}, nil)
		require.NoError(s.T(), err)
		config, err := s.Agent.Configuration()
		require.NoError(s.T(), err)
		// Convert extra_tags to a slice of strings
		extraTags := config["extra_tags"].([]interface{})
		extraTagsStrings := make([]string, len(extraTags))
		for i, tag := range extraTags {
			var ok bool
			extraTagsStrings[i], ok = tag.(string)
			require.True(s.T(), ok, "tag %d is not a string", i)
		}
		require.Equal(s.T(), []string{fmt.Sprintf("debug:step-%d", i)}, extraTagsStrings)
		err = s.Backend.PromoteConfigExperiment()
		require.NoError(s.T(), err)

		config, err = s.Agent.Configuration()
		require.NoError(s.T(), err)
		// Convert extra_tags to a slice of strings
		extraTags = config["extra_tags"].([]interface{})
		extraTagsStrings = make([]string, len(extraTags))
		for i, tag := range extraTags {
			var ok bool
			extraTagsStrings[i], ok = tag.(string)
			require.True(s.T(), ok, "tag %d is not a string", i)
		}
		require.Equal(s.T(), []string{fmt.Sprintf("debug:step-%d", i)}, extraTagsStrings)
	}
}

// TestConfigJQReplaceTag exercises the jq config operation with a realistic
// tag-replace scenario: an existing datadog.yaml carries a set of tags, and a jq
// transform rewrites the staging environment tag to production while leaving the
// other tags untouched. The new environment value is supplied as a typed jq
// argument rather than being baked into the transform text.
func (s *configSuite) TestConfigJQReplaceTag() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Seed a realistic multi-tag config.
	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "jq-seed-tags",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`{"tags": ["env:staging", "team:fleet", "service:installer"]}`),
			},
		},
	}, nil)
	require.NoError(s.T(), err)
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	// Replace the env:staging tag with the production value, leaving every other
	// tag in place. Both the matched value ($old) and the replacement ($new) are
	// supplied as typed jq arguments.
	//
	// The transform is written without spaces or string literals on purpose: the
	// daemon command is dispatched through PowerShell on Windows, and PowerShell
	// 5.1 cannot pass an argument that contains both spaces and double quotes to a
	// native executable (it re-quotes the argument and the embedded quotes break
	// it). Keeping the transform free of spaces means the serialized operations
	// JSON stays compact, so it survives the PowerShell -> installer.exe handoff.
	// Any jq transform exercised on Windows must observe the same constraint.
	err = s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "jq-replace-tag",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationJQ,
				FilePath:          "/datadog.yaml",
				Transform:         `.tags|=map(if(.==$old)then($new)else(.)end)`,
				Arguments:         []byte(`{"old":"env:staging","new":"env:prod"}`),
			},
		},
	}, nil)
	require.NoError(s.T(), err)

	assertTags := func(config map[string]any) {
		rawTags, ok := config["tags"].([]interface{})
		require.True(s.T(), ok, "tags should be a list")
		tags := make([]string, len(rawTags))
		for i, tag := range rawTags {
			tags[i], ok = tag.(string)
			require.True(s.T(), ok, "tag %d is not a string", i)
		}
		require.ElementsMatch(s.T(), []string{"env:prod", "team:fleet", "service:installer"}, tags)
	}

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	assertTags(config)

	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	assertTags(config)
}

func (s *configSuite) TestConfigFailureCrash() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)}},
	}, nil)
	require.NoError(s.T(), err)

	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "info", config["log_level"])
}

func (s *configSuite) TestConfigFailureTimeout() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.Agent.MustSetExperimentTimeout(60 * time.Second)
	defer s.Agent.MustUnsetExperimentTimeout()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
	}, nil)
	require.NoError(s.T(), err)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])

	time.Sleep(60 * time.Second)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		config, err := s.Agent.Configuration()
		require.NoError(c, err)
		require.Equal(c, "info", config["log_level"])
	}, 60*time.Second, 5*time.Second)
}

func (s *configSuite) TestConfigFailureHealth() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
	}, nil)
	require.NoError(s.T(), err)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])

	err = s.Backend.StopConfigExperiment()
	require.NoError(s.T(), err)
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "info", config["log_level"])
}

func (s *configSuite) TestConfigFilePermissions() {
	// Skip on Windows as POSIX permissions don't apply
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		s.T().Skip("Skipping file permission test on Windows (POSIX permissions not applicable)")
	}

	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Configure multiple files with different permission requirements
	nginxConfig := `{
		"init_config": {},
		"instances": [
			{
				"nginx_status_url": "http://localhost:8080/status"
			}
		]
	}`

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "file-permissions",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`{"log_level": "debug"}`),
			},
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/application_monitoring.yaml",
				Patch:             []byte(`{"enabled": true}`),
			},
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/conf.d/nginx.yaml",
				Patch:             []byte(nginxConfig),
			},
		},
	}, nil)
	require.NoError(s.T(), err)

	// Check datadog.yaml permissions (should be restricted: 640)
	datadogPerms, err := s.Host.GetFilePermissions("/etc/datadog-agent-exp/datadog.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "640", datadogPerms.Mode, "datadog.yaml should be restricted (640)")
	assert.Equal(s.T(), "dd-agent", datadogPerms.Owner, "datadog.yaml should be owned by dd-agent")
	assert.Equal(s.T(), "dd-agent", datadogPerms.Group, "datadog.yaml should have group dd-agent")

	// Check application_monitoring.yaml permissions (should be world-readable: 644)
	appMonPerms, err := s.Host.GetFilePermissions("/etc/datadog-agent-exp/application_monitoring.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "644", appMonPerms.Mode, "application_monitoring.yaml should be world-readable (644)")
	assert.Equal(s.T(), "root", appMonPerms.Owner, "application_monitoring.yaml should be owned by root")
	assert.Equal(s.T(), "root", appMonPerms.Group, "application_monitoring.yaml should have group root")

	// Check conf.d/nginx.yaml permissions (should be restricted: 640)
	nginxPerms, err := s.Host.GetFilePermissions("/etc/datadog-agent-exp/conf.d/nginx.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "640", nginxPerms.Mode, "integration configs should be restricted (640)")
	assert.Equal(s.T(), "dd-agent", nginxPerms.Owner, "integration configs should be owned by dd-agent")
	assert.Equal(s.T(), "dd-agent", nginxPerms.Group, "integration configs should have group dd-agent")

	// Promote and verify permissions persist
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	// Verify permissions after promotion
	datadogPerms, err = s.Host.GetFilePermissions("/etc/datadog-agent/datadog.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "640", datadogPerms.Mode)
	assert.Equal(s.T(), "dd-agent", datadogPerms.Owner)
	assert.Equal(s.T(), "dd-agent", datadogPerms.Group)

	appMonPerms, err = s.Host.GetFilePermissions("/etc/datadog-agent/application_monitoring.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "644", appMonPerms.Mode)
	assert.Equal(s.T(), "root", appMonPerms.Owner)
	assert.Equal(s.T(), "root", appMonPerms.Group)

	nginxPerms, err = s.Host.GetFilePermissions("/etc/datadog-agent/conf.d/nginx.yaml")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "640", nginxPerms.Mode)
	assert.Equal(s.T(), "dd-agent", nginxPerms.Owner)
	assert.Equal(s.T(), "dd-agent", nginxPerms.Group)
}

func (s *configSuite) TestConfigWithSecrets() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "123",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "SEC[log_level]"}`)}},
	}, map[string]string{
		"log_level": "WARN",
	})
	require.NoError(s.T(), err)
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "WARN", config["log_level"])
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "WARN", config["log_level"])
}

func (s *configSuite) TestSystemProbeConfig() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Configure system-probe settings with runtime security
	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "system-probe-config",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/system-probe.yaml",
				Patch:             []byte(`{"runtime_security_config": {"enabled": true}}`),
			},
		},
	}, nil)
	require.NoError(s.T(), err)

	// Check agent is alive during experiment
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		require.NoError(c, err, "agent should be running during experiment")
		require.NotEmpty(c, status.AgentMetadata.AgentVersion, "agent version should be available during experiment")
	}, 60*time.Second, 5*time.Second)

	// Promote the experiment
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	// Check agent is alive after promotion to stable
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		require.NoError(c, err, "agent should be running after promotion to stable")
		require.NotEmpty(c, status.AgentMetadata.AgentVersion, "agent version should be available after promotion")
	}, 60*time.Second, 5*time.Second)
}

// TestExperimentIntegrationLoaded verifies that an integration config deployed
// via the config experiment flow is picked up by the experiment agent before promotion.
func (s *configSuite) TestExperimentIntegrationLoaded() {
	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
		s.T().Skip("Skipping on Windows: experiment agent config paths are Linux-specific")
	}

	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	nginxConfig := `{"init_config": {}, "instances": [{"nginx_status_url": "http://localhost:8080/nginx_status"}]}`
	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "integration-loaded-test",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/datadog.yaml",
				Patch:             []byte(`{}`),
			},
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/conf.d/nginx.yaml",
				Patch:             []byte(nginxConfig),
			},
		},
	}, nil)
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		require.NoError(c, err)
		_, nginxLoaded := status.RunnerStats.Checks["nginx"]
		assert.True(c, nginxLoaded, "nginx check should be loaded from the experiment conf.d directory")
	}, 60*time.Second, 5*time.Second)

	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		require.NoError(c, err)
		_, nginxLoaded := status.RunnerStats.Checks["nginx"]
		assert.True(c, nginxLoaded, "nginx check should still be loaded after promotion")
	}, 60*time.Second, 5*time.Second)
}

// TestConfigRollbackDeploymentID tests that rolling back a config experiment
// correctly preserves the stable_config_version and does not overwrite it
// with the experiment deployment ID.
func (s *configSuite) TestConfigRollbackDeploymentID() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Get initial remote config state
	initialState, err := s.Backend.RemoteConfigStatus()
	require.NoError(s.T(), err)

	// Find the datadog-agent package state
	var initialAgentState *backend.RemoteConfigStatePackage
	for i, pkg := range initialState.Packages {
		if pkg.Package == "datadog-agent" {
			initialAgentState = &initialState.Packages[i]
			break
		}
	}
	require.NotNil(s.T(), initialAgentState, "datadog-agent package should be in remote config state")
	initialStableConfigVersion := initialAgentState.StableConfigVersion

	// Start a config experiment with a known deployment ID
	experimentDeploymentID := "test-rollback-deployment-123"
	err = s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: experimentDeploymentID,
		FileOperations: []backend.FileOperation{
			{FileOperationType: backend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)},
		},
	}, nil)
	require.NoError(s.T(), err)

	// Verify config changed during experiment
	config, err := s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "debug", config["log_level"])

	// Verify remote config state shows experiment_config_version
	experimentState, err := s.Backend.RemoteConfigStatus()
	require.NoError(s.T(), err)

	var agentStateAfterStart *backend.RemoteConfigStatePackage
	for i, pkg := range experimentState.Packages {
		if pkg.Package == "datadog-agent" {
			agentStateAfterStart = &experimentState.Packages[i]
			break
		}
	}
	require.NotNil(s.T(), agentStateAfterStart, "datadog-agent package should be in remote config state")
	require.Equal(s.T(), experimentDeploymentID, agentStateAfterStart.ExperimentConfigVersion,
		"experiment_config_version should be set after StartConfigExperiment")
	require.Equal(s.T(), initialStableConfigVersion, agentStateAfterStart.StableConfigVersion,
		"stable_config_version should not change during experiment")

	// Stop the experiment (rollback)
	err = s.Backend.StopConfigExperiment()
	require.NoError(s.T(), err)

	// Verify config rolled back
	config, err = s.Agent.Configuration()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "info", config["log_level"])

	// Verify remote config state after rollback
	rollbackState, err := s.Backend.RemoteConfigStatus()
	require.NoError(s.T(), err)

	var agentStateAfterRollback *backend.RemoteConfigStatePackage
	for i, pkg := range rollbackState.Packages {
		if pkg.Package == "datadog-agent" {
			agentStateAfterRollback = &rollbackState.Packages[i]
			break
		}
	}
	require.NotNil(s.T(), agentStateAfterRollback, "datadog-agent package should be in remote config state")

	// Verify stable_config_version is preserved after rollback
	require.Equal(s.T(), initialStableConfigVersion, agentStateAfterRollback.StableConfigVersion,
		"stable_config_version should not change after rollback, but got: %s (expected: %s)",
		agentStateAfterRollback.StableConfigVersion, initialStableConfigVersion)
	require.Empty(s.T(), agentStateAfterRollback.ExperimentConfigVersion,
		"experiment_config_version should be empty after rollback")
}

// DDOT config-experiment tests: install the DDOT extension on the datadog-agent package and drive
// config experiments that patch /otel-config.yaml, verifying the collector is relaunched with the
// experiment config during the experiment and with the promoted config after promotion.

// ddotCardinalityPatch flips processors.infraattributes.cardinality (2 -> 1) in the DDOT collector
// config. It is a benign, valid change that keeps the collector pipeline healthy.
const ddotCardinalityPatch = `{"processors":{"infraattributes":{"cardinality":1}}}`

// ddotBrokenPatch sets an invalid debug-exporter verbosity, which the collector rejects at startup.
// It breaks DDOT without affecting the core agent (which does not read otel-config.yaml).
const ddotBrokenPatch = `{"exporters":{"debug":{"verbosity":"not-a-valid-verbosity"}}}`

// TestDDOTConfigUpdateAndPromote installs DDOT, updates its otel-config.yaml through a config
// experiment, and verifies the collector runs the experiment config during the experiment and the
// promoted config after promotion.
func (s *configSuite) TestDDOTConfigUpdateAndPromote() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(getAgentPackageURL(s.T(), ""), "ddot")
	defer func() { _, _ = s.Installer.RemoveExtension("datadog-agent", "ddot") }()
	verifyDDOTRunning(s.T(), s.Agent)

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "ddot-config-1",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/otel-config.yaml", Patch: []byte(ddotCardinalityPatch)}},
	}, nil)
	s.Require().NoError(err)

	// The experiment config carries the patch and the collector is relaunched reading it.
	s.Require().Contains(s.readActiveOTelConfig(true), "cardinality: 1")
	verifyDDOTRunning(s.T(), s.Agent)
	s.assertCollectorConfigPath(true)

	err = s.Backend.PromoteConfigExperiment()
	s.Require().NoError(err)

	// After promotion the collector runs the promoted config from the stable location.
	s.Require().Contains(s.readActiveOTelConfig(false), "cardinality: 1")
	verifyDDOTRunning(s.T(), s.Agent)
	s.assertCollectorConfigPath(false)
}

// TestDDOTConfigUpdateRollback verifies that stopping a DDOT config experiment restores the stable
// otel-config.yaml and the collector runs it again.
func (s *configSuite) TestDDOTConfigUpdateRollback() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(getAgentPackageURL(s.T(), ""), "ddot")
	defer func() { _, _ = s.Installer.RemoveExtension("datadog-agent", "ddot") }()
	verifyDDOTRunning(s.T(), s.Agent)

	stableBefore := s.readActiveOTelConfig(false)

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "ddot-config-rollback",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/otel-config.yaml", Patch: []byte(ddotCardinalityPatch)}},
	}, nil)
	s.Require().NoError(err)
	s.Require().Contains(s.readActiveOTelConfig(true), "cardinality: 1")
	verifyDDOTRunning(s.T(), s.Agent)

	err = s.Backend.StopConfigExperiment()
	s.Require().NoError(err)

	// The stable otel-config.yaml is restored byte-for-byte and the collector is healthy on it again.
	s.Require().Equal(stableBefore, s.readActiveOTelConfig(false))
	verifyDDOTRunning(s.T(), s.Agent)
	s.assertCollectorConfigPath(false)
}

// TestDDOTConfigBadConfigRollsBack verifies that a config experiment carrying a broken
// otel-config.yaml reaches the experiment collector (breaking it) and that stopping the experiment
// restores a healthy collector on the stable config. This exercises the failure path: the broken
// config must actually be applied to the running DDOT, proving the experiment collector reads the
// experiment config rather than the stable one.
func (s *configSuite) TestDDOTConfigBadConfigRollsBack() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	s.Installer.MustInstallExtension(getAgentPackageURL(s.T(), ""), "ddot")
	defer func() { _, _ = s.Installer.RemoveExtension("datadog-agent", "ddot") }()
	verifyDDOTRunning(s.T(), s.Agent)

	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID:   "ddot-config-broken",
		FileOperations: []backend.FileOperation{{FileOperationType: backend.FileOperationMergePatch, FilePath: "/otel-config.yaml", Patch: []byte(ddotBrokenPatch)}},
	}, nil)
	s.Require().NoError(err)

	// The broken experiment config reaches DDOT and prevents the collector from running.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		assert.NoError(c, err)
		assert.True(c, status.OtelAgent.Error != "" || status.OtelAgent.AgentVersion == "",
			"DDOT should be unhealthy while running the broken experiment config")
	}, 2*time.Minute, 5*time.Second)

	err = s.Backend.StopConfigExperiment()
	s.Require().NoError(err)

	// Rolling back restores the stable config and a healthy collector.
	s.Require().NotContains(s.readActiveOTelConfig(false), "not-a-valid-verbosity")
	verifyDDOTRunning(s.T(), s.Agent)
}

// readActiveOTelConfig returns the contents of the otel-config.yaml the collector is (or will be)
// running. On Linux the experiment config lives in a separate directory (/etc/datadog-agent-exp);
// on Windows config experiments are applied in place to the stable data directory (the -exp
// directory only holds the rollback backup), so the active config is always at the stable path.
func (s *configSuite) readActiveOTelConfig(experiment bool) string {
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		path := "/etc/datadog-agent/otel-config.yaml"
		if experiment {
			path = "/etc/datadog-agent-exp/otel-config.yaml"
		}
		out, err := s.Env().RemoteHost.Execute("sudo cat " + path)
		s.Require().NoError(err)
		return out
	case e2eos.WindowsFamily:
		out, err := s.Env().RemoteHost.Execute(`Get-Content -Raw 'C:\ProgramData\Datadog\otel-config.yaml'`)
		s.Require().NoError(err)
		return out
	default:
		s.T().Fatalf("unsupported OS family: %v", s.Env().RemoteHost.OSFamily)
		return ""
	}
}

// assertCollectorConfigPath asserts (on Linux, where DDOT runs under dd-procmgr) that the running
// otel-agent process was launched with the experiment or stable otel-config.yaml as its --config.
// This is the direct proof that the experiment collector reads the experiment config. On Windows the
// in-place config model means the collector always reads the stable path, so only DDOT liveness and
// file contents are asserted (in the callers).
func (s *configSuite) assertCollectorConfigPath(experiment bool) {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		return
	}
	want := "/etc/datadog-agent/otel-config.yaml"
	if experiment {
		want = "/etc/datadog-agent-exp/otel-config.yaml"
	}
	ddot.AssertDDOTManagedByProcmgr(s.T(), s.Env().RemoteHost)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute(`sudo ps -eo args | grep '[o]tel-agent' | grep -o -- '--config [^ ]*' | head -1`)
		assert.NoError(c, err)
		assert.Equal(c, "--config "+want, strings.TrimSpace(out),
			"the running collector should read %s", want)
	}, 2*time.Minute, 5*time.Second)
}
