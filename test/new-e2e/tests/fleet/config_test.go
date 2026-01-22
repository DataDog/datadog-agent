// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fleet contains tests for fleet
package fleet

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
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
	suite.Run(t, newConfigSuite, suite.AllPlatforms)
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
	status, err := s.Agent.Status()
	require.NoError(s.T(), err, "agent should be running during experiment")
	require.NotEmpty(s.T(), status.AgentMetadata.AgentVersion, "agent version should be available during experiment")

	// Promote the experiment
	err = s.Backend.PromoteConfigExperiment()
	require.NoError(s.T(), err)

	// Check agent is alive after promotion to stable
	status, err = s.Agent.Status()
	require.NoError(s.T(), err, "agent should be running after promotion to stable")
	require.NotEmpty(s.T(), status.AgentMetadata.AgentVersion, "agent version should be available after promotion")
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
