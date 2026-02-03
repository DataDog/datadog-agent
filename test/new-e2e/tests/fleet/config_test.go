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

func (s *configSuite) TestConfigIntegration() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	apacheConfig := `{
		"init_config": {},
		"instances": [
			{
				"apache_status_url": "http://localhost/server-status?auto"
			}
		]
	}`
	err := s.Backend.StartConfigExperiment(backend.ConfigOperations{
		DeploymentID: "apache-integration",
		FileOperations: []backend.FileOperation{
			{
				FileOperationType: backend.FileOperationMergePatch,
				FilePath:          "/conf.d/apache.yaml",
				Patch:             []byte(apacheConfig),
			},
		},
	})
	require.NoError(s.T(), err)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		status, err := s.Agent.Status()
		require.NoError(c, err)
		require.Contains(c, status.RunnerStats.Checks, "apache")
	}, 30*time.Second, 5*time.Second)
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
