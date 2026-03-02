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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

const thirdPartyIntegration = "datadog-ping==1.0.2"

type upgradeSuite struct {
	suite.FleetSuite
}

func newUpgradeSuite() e2e.Suite[environments.Host] {
	return &upgradeSuite{}
}

func TestFleetUpgrade(t *testing.T) {
	suite.Run(t, newUpgradeSuite, suite.AllPlatforms)
}

func (s *upgradeSuite) TestIntegrationPreservationDuringExperiment() {
	// Install the testing pipeline version (our code) as stable, so preStartExperiment
	// runs from the binary under test when the experiment is started.
	s.Agent.MustInstall(agent.WithRemoteUpdates())
	defer s.Agent.MustUninstall()

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before experiment")

	// Experiment with the released stable version; preStartExperiment runs from our binary.
	// If the stable version equals the installed testing version, the installer would reject it
	// ("cannot set new experiment to the same version as stable"), so fall back to the previous minor.
	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	targetVersion := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	if targetVersion == testingVersion {
		targetVersion = s.Backend.Catalog().LatestMinus(backend.BranchStable, "datadog-agent", 1)
	}
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved in experiment")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved after promotion")
}

func (s *upgradeSuite) TestUpgradeFailureTimeout() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.Agent.MustSetExperimentTimeout(60 * time.Second)
	defer s.Agent.MustUnsetExperimentTimeout()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	version, err := s.Agent.Version()
	s.Require().NoError(err)
	s.Require().NotEqual(originalVersion, version)

	time.Sleep(90 * time.Second)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 300*time.Second, 30*time.Second)
}

func (s *upgradeSuite) TestUpgradeFailureHealth() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()

	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	version, err := s.Agent.Version()
	s.Require().NoError(err)
	s.Require().NotEqual(originalVersion, version)

	err = s.Backend.StopExperiment("datadog-agent")
	s.Require().NoError(err)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 300*time.Second, 30*time.Second)
}
