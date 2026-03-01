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

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
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

// TestODBCConfigPreservedOnUpgrade verifies that customer-modified ODBC config files
// (odbc.ini and odbcinst.ini) in the agent's embedded/etc/ directory survive a
// Fleet Automation upgrade. This covers the SQL Server DBM use case where customers
// register ODBC drivers by editing those files after the agent is installed.
func (s *upgradeSuite) TestODBCConfigPreservedOnUpgrade() {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		s.T().Skip("ODBC config preservation is only relevant on Linux")
	}
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	_, err := s.Env().RemoteHost.Execute(`sudo sh -c 'printf "[ODBC]\nTrace=no\n" > /opt/datadog-agent/embedded/etc/odbc.ini'`)
	s.Require().NoError(err)
	_, err = s.Env().RemoteHost.Execute(`sudo sh -c 'printf "[ODBC Driver 18 for SQL Server]\nDescription=Microsoft ODBC Driver 18 for SQL Server\nDriver=/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.6.so.1.1\nUsageCount=1\n" > /opt/datadog-agent/embedded/etc/odbcinst.ini'`)
	s.Require().NoError(err)
	
	targetVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)

	odbcIni, err := s.Env().RemoteHost.Execute("sudo cat /opt/datadog-agent/embedded/etc/odbc.ini")
	s.Require().NoError(err)
	s.Require().Contains(odbcIni, "[ODBC]")
	odbcInst, err := s.Env().RemoteHost.Execute("sudo cat /opt/datadog-agent/embedded/etc/odbcinst.ini")
	s.Require().NoError(err)
	s.Require().Contains(odbcInst, "[ODBC Driver 18 for SQL Server]")
}
