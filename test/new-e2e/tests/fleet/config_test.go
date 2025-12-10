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
	})
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
		})
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
	})
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
	})
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
	})
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
