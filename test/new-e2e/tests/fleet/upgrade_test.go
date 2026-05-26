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

const thirdPartyIntegration = "datadog-ping==1.0.2"

type upgradeSuite struct {
	suite.FleetSuite
}

// snapshotIntegrationState dumps the on-host state that determines whether
// integration save/restore worked: the installer journal (where the
// save_custom_integrations / restore_custom_integrations spans land), the
// contents of /opt/datadog-packages/tmp/ (where pre.py writes and post.py
// reads the diff file), and the stable site-packages directory listing.
// Captured to T.Logf at every upgrade phase boundary so that a red CI test
// can be triaged from the log alone, without SSH. Linux-only; no-op on
// other OS families.
func (s *upgradeSuite) snapshotIntegrationState(phase string) {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		return
	}
	s.T().Logf("=== integration state snapshot: %s ===", phase)
	cmds := []struct {
		label   string
		command string
	}{
		{"installer journal (last 200 lines)", "sudo journalctl -u 'datadog-agent-installer*.service' --since=-15m --no-pager --output=cat 2>&1 | tail -200"},
		{"/opt/datadog-packages/tmp listing", "sudo ls -la /opt/datadog-packages/tmp/ 2>&1"},
		{".diff_python_installed_packages.txt", "sudo cat /opt/datadog-packages/tmp/.diff_python_installed_packages.txt 2>&1 || echo NOT_FOUND"},
		{".post_python_installed_packages.txt", "sudo cat /opt/datadog-packages/tmp/.post_python_installed_packages.txt 2>&1 || echo NOT_FOUND"},
		{"stable site-packages (datadog_* only)", "sudo ls /opt/datadog-packages/datadog-agent/stable/embedded/lib/python*/site-packages/ 2>/dev/null | grep -E '^datadog' || echo NONE"},
	}
	for _, c := range cmds {
		out, err := s.Env().RemoteHost.Execute(c.command)
		if err != nil {
			s.T().Logf("[%s] %s: exec error: %v\n%s", phase, c.label, err, out)
			continue
		}
		s.T().Logf("[%s] %s:\n%s", phase, c.label, out)
	}
}

func newUpgradeSuite() e2e.Suite[environments.Host] {
	return &upgradeSuite{}
}

func TestFleetUpgrade(t *testing.T) {
	suite.Run(t, newUpgradeSuite, suite.Platforms())
}

func (s *upgradeSuite) TestUpgrade() {
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
	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, targetVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)
}

func (s *upgradeSuite) TestIntegrationPreservationDebToOCI() {
	s.Agent.MustInstall(agent.WithRemoteUpdates())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("DebToOCI: after MustInstall (pipeline DEB stable)")

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("DebToOCI: after InstallIntegration")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before experiment")

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	targetVersion := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	if targetVersion == testingVersion {
		targetVersion = s.Backend.Catalog().LatestMinus(backend.BranchStable, "datadog-agent", 1)
	}
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("DebToOCI: after StartExperiment to released OCI stable")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved in experiment")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("DebToOCI: after PromoteExperiment")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved after promotion")
}

// TestIntegrationPreservationOCIToOCI tests that integrations are preserved during an OCI→OCI upgrade.
// It first installs a stable DEB, then promotes to the pipeline's OCI version to reach an OCI-stable
// state, then experiments from that OCI version to another OCI version and verifies the integration
// is preserved throughout.
func (s *upgradeSuite) TestIntegrationPreservationOCIToOCI() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("OCIToOCI: after MustInstall (released OCI stable)")

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err := s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("OCIToOCI: after preparatory promote to pipeline testing")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	err = s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("OCIToOCI: after InstallIntegration on pipeline stable")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before OCI experiment")

	targetVersion := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	if targetVersion == testingVersion {
		targetVersion = s.Backend.Catalog().LatestMinus(backend.BranchStable, "datadog-agent", 1)
	}
	err = s.Backend.StartExperiment("datadog-agent", targetVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("OCIToOCI: after StartExperiment to released stable")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved in OCI experiment")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("OCIToOCI: after PromoteExperiment")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved after OCI promotion")
}

// TestIntegrationPreservationStableToOCIExperiment verifies that a third-party integration
// installed against a released OCI stable survives an experiment to the pipeline's testing
// version and the subsequent promotion. This exercises the released stable's preStartExperiment
// (old save) handing off to the pipeline build's postStartExperiment (new restore), which is the
// production upgrade path customers follow. The existing TestIntegrationPreservationOCIToOCI
// inverts that direction by promoting the pipeline build to stable first.
func (s *upgradeSuite) TestIntegrationPreservationStableToOCIExperiment() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("StableToOCIExperiment: after MustInstall (released OCI stable)")

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("StableToOCIExperiment: after InstallIntegration")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed on released stable before experiment")

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("StableToOCIExperiment: after StartExperiment to pipeline testing")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved in experiment to pipeline version")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("StableToOCIExperiment: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved after promotion to pipeline version")
}

// TestIntegrationPreservationOnExperimentRollback verifies that a third-party integration
// installed on the released OCI stable survives an experiment that gets stopped and rolled
// back. Mirrors TestUpgradeFailureHealth but installs the integration first and asserts it
// is still present on the (now reverted) stable after rollback. Locks in that
// preStartExperiment must not destructively touch the stable's site-packages when only
// saving for the experiment.
func (s *upgradeSuite) TestIntegrationPreservationOnExperimentRollback() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("Rollback: after MustInstall (released OCI stable)")

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("Rollback: after InstallIntegration")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before experiment")

	originalVersion, err := s.Agent.Version()
	s.Require().NoError(err)

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("Rollback: after StartExperiment to pipeline testing")

	version, err := s.Agent.Version()
	s.Require().NoError(err)
	s.Require().NotEqual(originalVersion, version, "experiment should be running before rollback")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved while experiment is running")

	err = s.Backend.StopExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("Rollback: after StopExperiment (rollback triggered)")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 300*time.Second, 30*time.Second)
	s.snapshotIntegrationState("Rollback: after rollback completed")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should still be installed on the (reverted) stable after rollback")
}

// TestIntegrationPreservationMultiHop verifies that a third-party integration installed
// on the initial released stable survives two consecutive experiment-and-promote cycles
// to different targets. This validates that .post_python_installed_packages.txt is
// correctly refreshed after each promote so the next experiment's diff is computed
// against the right baseline. A broken refresh would either drop the integration from
// the diff (silent loss) or carry stale entries that the new agent can't reinstall.
func (s *upgradeSuite) TestIntegrationPreservationMultiHop() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("MultiHop: after MustInstall (released OCI stable)")

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("MultiHop: after InstallIntegration")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before any hop")

	// Hop 1: released stable -> pipeline testing -> promote.
	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("MultiHop: after StartExperiment hop 1 (testing)")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("MultiHop: after PromoteExperiment hop 1")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should be preserved after hop 1 promote")

	// Hop 2: now-stable pipeline testing -> a different released stable -> promote.
	hop2Target := s.Backend.Catalog().Latest(backend.BranchStable, "datadog-agent")
	if hop2Target == testingVersion {
		hop2Target = s.Backend.Catalog().LatestMinus(backend.BranchStable, "datadog-agent", 1)
	}
	err = s.Backend.StartExperiment("datadog-agent", hop2Target)
	s.Require().NoError(err)
	s.snapshotIntegrationState("MultiHop: after StartExperiment hop 2")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("MultiHop: after PromoteExperiment hop 2")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, hop2Target, packageVersion)
	}, 300*time.Second, 30*time.Second)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "integration should still be preserved after hop 2 promote")
}

// secondaryThirdPartyIntegration is a second third-party integration used alongside
// datadog-ping in TestIntegrationPreservationMixedCustomization to exercise the
// multi-package branch of omnibus/python-scripts/packages.py:create_diff_installed_packages_file.
// Must be an extras-only integration (i.e., not in requirements-agent-release.txt) so
// `agent integration install` does not refuse it via the min-version guard at
// cmd/agent/subcommands/integrations/command.go:453.
//
// TODO(blind-spots #5): the version-upgrade branch of create_diff_installed_packages_file
// (customer upgrades a *bundled* integration to a newer version) is still not covered.
// Requires picking a core integration whose newer wheel is also published on the extras
// feed, because post.py:install_datadog_package hardcodes `-t` (extras layout) on
// restore. Tracked separately.
const secondaryThirdPartyIntegration = "datadog-puma==2.0.0"

// TestIntegrationPreservationMixedCustomization verifies that two third-party
// integrations installed on the released stable both survive an experiment-and-promote
// to the pipeline testing version. This exercises the multi-package-diff branch of
// create_diff_installed_packages_file: a single .diff_python_installed_packages.txt
// containing multiple entries, all of which must be reinstalled by post.py.
func (s *upgradeSuite) TestIntegrationPreservationMixedCustomization() {
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState("MixedCustomization: after MustInstall (released OCI stable)")

	err := s.Agent.InstallIntegration(thirdPartyIntegration)
	s.Require().NoError(err)
	err = s.Agent.InstallIntegration(secondaryThirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState("MixedCustomization: after both integration installs")

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "ping should be installed before experiment")
	s.Require().Equal("2.0.0", installedIntegrations["puma"], "puma should be installed before experiment")

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState("MixedCustomization: after StartExperiment to pipeline testing")

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "ping should be preserved in experiment")
	s.Assert().Equal("2.0.0", installedIntegrations["puma"], "puma should be preserved in experiment")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("MixedCustomization: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	installedIntegrations, err = s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Assert().Equal("1.0.2", installedIntegrations["ping"], "ping should be preserved after promotion")
	s.Assert().Equal("2.0.0", installedIntegrations["puma"], "puma should be preserved after promotion")
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
	// TODO: This test uses a DEB→OCI upgrade (testing pipeline DEB as stable, testing
	// pipeline OCI as experiment) because the ODBC save/restore code is new and not yet
	// in a released stable binary. Once this fix ships as a stable release, rewrite this
	// test to use WithStablePackages() + BranchTesting experiment, like other upgrade tests.
	s.Agent.MustInstall(agent.WithRemoteUpdates())
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

	// After DEB→OCI promote, the agent runs from /opt/datadog-packages/datadog-agent/stable/
	odbcIni, err := s.Env().RemoteHost.Execute("sudo cat /opt/datadog-packages/datadog-agent/stable/embedded/etc/odbc.ini")
	s.Require().NoError(err)
	s.Require().Contains(odbcIni, "[ODBC]")
	odbcInst, err := s.Env().RemoteHost.Execute("sudo cat /opt/datadog-packages/datadog-agent/stable/embedded/etc/odbcinst.ini")
	s.Require().NoError(err)
	s.Require().Contains(odbcInst, "[ODBC Driver 18 for SQL Server]")
}
