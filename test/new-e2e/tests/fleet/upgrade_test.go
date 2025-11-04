// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

type upgradeSuite struct {
	suite.FleetSuite
}

func newUpgradeSuite() e2e.Suite[environments.Host] {
	return &upgradeSuite{}
}

func TestFleetUpgrade(t *testing.T) {
	suite.Run(t, newUpgradeSuite, suite.AllPlatforms)
}

func (s *upgradeSuite) TestUpgradeFailureTimeout() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()
	s.Agent.MustSetExperimentTimeout(60 * time.Second)
	defer s.Agent.MustUnsetExperimentTimeout()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	version, err := s.Agent.Version()
	s.Require().NoError(err)
	s.Require().Equal(targetVersion, version)

	time.Sleep(90 * time.Second)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 120*time.Second, 30*time.Second)
}

func (s *upgradeSuite) TestUpgradeFailureHealth() {
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	version, err := s.Agent.Version()
	s.Require().NoError(err)
	s.Require().Equal(targetVersion, version)

	err = s.Backend.StopExperiment("datadog-agent")
	s.Require().NoError(err)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 120*time.Second, 30*time.Second)
}

// type postUpgradeSuite struct {
// 	testsuite.Suite

// 	host                 *environments.Host
// 	agent                *agent.Agent
// 	expectedVersion      string
// 	expectedIntegrations []string
// }

// func (s *postUpgradeSuite) TestStatus() {
// 	status, err := s.agent.Status()
// 	s.Require().NoError(err)

// 	s.Equal(s.expectedVersion, status.AgentMetadata.AgentVersion)
// 	s.NotZero(status.ApmStats.Pid)
// 	s.True(status.FleetAutomationStatus.InstallerRunning)
// }

// func (s *postUpgradeSuite) TestSingleInstall() {
// 	agentReleaseFiles, err := s.host.RemoteHost.FindFiles("requirements-agent-release.txt")
// 	s.Require().NoError(err)

// 	s.Require().Len(agentReleaseFiles, 1, "should find exactly one requirements-agent-release.txt file, found %d.\npaths: %v", len(agentReleaseFiles), agentReleaseFiles)
// }

// func (s *postUpgradeSuite) TestIntegrationsPersisted() {
// 	integrations, err := s.agent.InstalledIntegrations()
// 	s.Require().NoError(err)

// 	for _, integration := range s.expectedIntegrations {
// 		s.Contains(integrations, integration, "expected integration %s to be installed", integration)
// 	}
// }
