// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

const thirdPartyIntegration = "datadog-ping==1.0.2"

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

type preservationSuite struct {
	suite.FleetSuite
}

func newPreservationSuite() e2e.Suite[environments.Host] {
	return &preservationSuite{}
}

// TestFleetIntegrationPreservation runs the integration-preservation suite on Linux only.
// All tests in this suite exercise Linux-specific behaviors (file ownership, pip install paths,
// chown correctness) that are not applicable on Windows.
func TestFleetIntegrationPreservation(t *testing.T) {
	suite.Run(t, newPreservationSuite, suite.Platforms())
}

// snapshotIntegrationState dumps the on-host state that determines whether
// integration save/restore worked: the installer journal (where the
// save_custom_integrations / restore_custom_integrations spans land), the
// contents of /opt/datadog-packages/tmp/ (where pre.py writes and post.py
// reads the diff file), and the stable site-packages directory listing.
// Captured to T.Logf at every upgrade phase boundary so that a red CI test
// can be triaged from the log alone, without SSH. Linux-only; no-op on
// other OS families.
func (s *preservationSuite) snapshotIntegrationState(phase string) {
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
		// Ownership of reinstalled integration files: root:root indicates post.py ran pip as root
		// without a subsequent chown, which blocks dd-agent from writing __pycache__ later.
		{"restored integration file ownership", "sudo find /opt/datadog-packages/datadog-agent/ -maxdepth 8 \\( -name 'datadog_ping-*.dist-info' -o -name 'datadog_puma-*.dist-info' -o -wholename '*/datadog_checks/ping' -o -wholename '*/datadog_checks/puma' \\) -printf '%u\\t%p\\n' 2>/dev/null || echo NONE"},
		{"integration show datadog-ping", "sudo -u dd-agent datadog-agent integration show datadog-ping 2>&1 || echo FAILED"},
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

// integrationDistInfoOwner returns the Unix owner of the named integration's dist-info
// directory under the given OCI package location ("experiment" or "stable").
// Returns empty string if the directory does not exist or the command fails.
// A "root" result indicates the files were installed by post.py running as root
// (failure mode 1: post.py calls pip without dropping privileges).
func (s *preservationSuite) integrationDistInfoOwner(location, integrationName string) string {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		return ""
	}
	out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`find /opt/datadog-packages/datadog-agent/%s/embedded/lib/ -maxdepth 5 -name 'datadog_%s-*.dist-info' -type d -printf '%%u\n' 2>/dev/null | sort -u`,
		location, integrationName,
	))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// integrationCheckDirOwner returns the Unix owner of the datadog_checks/<name> directory
// under the given OCI package location. If this directory is root-owned, dd-agent cannot
// create or refresh __pycache__ inside it, causing silent bytecode-cache misses on every
// import (failure mode 2: root-owned parent directory blocks dd-agent from writing .pyc).
func (s *preservationSuite) integrationCheckDirOwner(location, integrationName string) string {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		return ""
	}
	out, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`find /opt/datadog-packages/datadog-agent/%s/embedded/lib/ -maxdepth 6 -wholename '*/datadog_checks/%s' -type d -printf '%%u\n' 2>/dev/null | sort -u`,
		location, integrationName,
	))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (s *preservationSuite) TestIntegrationPreservationDebToOCI() {
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

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved in experiment")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after restoration in experiment")
	}, 60*time.Second, 5*time.Second)

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("DebToOCI: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved after promotion")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after promotion")
	}, 60*time.Second, 5*time.Second)
}

// TestIntegrationPreservationOCIToOCI tests that integrations are preserved during an OCI→OCI upgrade.
// It first installs a stable DEB, then promotes to the pipeline's OCI version to reach an OCI-stable
// state, then experiments from that OCI version to another OCI version and verifies the integration
// is preserved throughout.
func (s *preservationSuite) TestIntegrationPreservationOCIToOCI() {
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

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved in OCI experiment")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after restoration in OCI experiment")
	}, 60*time.Second, 5*time.Second)

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("OCIToOCI: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved after OCI promotion")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after OCI promotion")
	}, 60*time.Second, 5*time.Second)
}

// TestIntegrationPreservationStableToOCIExperiment verifies that a third-party integration
// installed against a released OCI stable survives an experiment to the pipeline's testing
// version and the subsequent promotion. This exercises the released stable's preStartExperiment
// (old save) handing off to the pipeline build's postStartExperiment (new restore), which is the
// production upgrade path customers follow. The existing TestIntegrationPreservationOCIToOCI
// inverts that direction by promoting the pipeline build to stable first.
func (s *preservationSuite) TestIntegrationPreservationStableToOCIExperiment() {
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

	// post.py reinstalls integrations asynchronously after the daemon restarts; poll until it finishes.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved in experiment to pipeline version")

		// Failure mode 4: integration show must succeed — if post.py's pip install failed silently
		// (run_command swallows CalledProcessError), dist-info is absent and show returns an error
		// while the check may still run from a cached sys.modules entry.
		showOut, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after restoration in experiment")
		assert.Contains(c, showOut, "1.0.2", "integration show should report the restored version")
	}, 60*time.Second, 5*time.Second)

	if s.Env().RemoteHost.OSFamily == e2eos.LinuxFamily {
		// Failure mode 1: dist-info files must be owned by dd-agent after restoration.
		// post.py calls pip as root (no privilege drop in executePythonScript / install_datadog_package),
		// so without a post-install chown the files land as root:root. installFilesystem chowns the
		// experiment tree before post.py runs, but pip overwrites ownership for the reinstalled package.
		s.Assert().Equal("dd-agent", s.integrationDistInfoOwner("experiment", "ping"),
			"dist-info should be owned by dd-agent after restoration; root ownership blocks future 'agent integration install' from unlinking root-owned .pyc files")

		// Failure mode 2: the datadog_checks/<name> directory must be dd-agent-owned so the running
		// agent can create and refresh __pycache__ inside it. If root-owned (mode 0755), dd-agent
		// has r-x but not w, so Python silently skips writing bytecode and re-parses source on every import.
		s.Assert().Equal("dd-agent", s.integrationCheckDirOwner("experiment", "ping"),
			"datadog_checks/ping should be owned by dd-agent after restoration; root ownership prevents dd-agent from writing __pycache__ entries")
	}

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("StableToOCIExperiment: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved after promotion to pipeline version")

		// Failure mode 4 (post-promote): integration show must still succeed after promotion.
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after promotion")
	}, 60*time.Second, 5*time.Second)
}

// runIntegrationOwnershipTest is the shared body for TestIntegrationPreservationRootInstall
// and TestIntegrationPreservationDDAgentInstall. It installs the agent, installs the
// third-party integration as installUser, runs a stable→pipeline-experiment→promote cycle,
// and asserts that the restored integration files are owned by dd-agent in both the
// experiment and stable locations.
func (s *preservationSuite) runIntegrationOwnershipTest(installUser, label string) {
	if s.Env().RemoteHost.OSFamily != e2eos.LinuxFamily {
		s.T().Skip("ownership checks require Linux: InstallIntegrationAs and find -printf are not available on other platforms")
	}
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackages())
	defer s.Agent.MustUninstall()
	s.snapshotIntegrationState(label + ": after MustInstall (released OCI stable)")

	err := s.Agent.InstallIntegrationAs(installUser, thirdPartyIntegration)
	s.Require().NoError(err)
	s.snapshotIntegrationState(label + ": after InstallIntegrationAs " + installUser)

	installedIntegrations, err := s.Agent.InstalledIntegrations()
	s.Require().NoError(err)
	s.Require().Equal("1.0.2", installedIntegrations["ping"], "integration should be installed before experiment")

	testingVersion := s.Backend.Catalog().Latest(backend.BranchTesting, "datadog-agent")
	err = s.Backend.StartExperiment("datadog-agent", testingVersion)
	s.Require().NoError(err)
	s.snapshotIntegrationState(label + ": after StartExperiment to pipeline testing")

	// post.py reinstalls integrations asynchronously after the daemon restarts; poll until it finishes.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved in experiment")

		showOut, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after restoration in experiment")
		assert.Contains(c, showOut, "1.0.2", "integration show should report the restored version")
	}, 60*time.Second, 5*time.Second)

	s.Assert().Equal("dd-agent", s.integrationDistInfoOwner("experiment", "ping"),
		"dist-info should be owned by dd-agent after restoration (installed as "+installUser+")")
	s.Assert().Equal("dd-agent", s.integrationCheckDirOwner("experiment", "ping"),
		"datadog_checks/ping should be owned by dd-agent after restoration (installed as "+installUser+")")

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState(label + ": after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved after promotion")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after promotion")
	}, 60*time.Second, 5*time.Second)

	s.Assert().Equal("dd-agent", s.integrationDistInfoOwner("stable", "ping"),
		"dist-info should be owned by dd-agent in stable after promotion (installed as "+installUser+")")
	s.Assert().Equal("dd-agent", s.integrationCheckDirOwner("stable", "ping"),
		"datadog_checks/ping should be owned by dd-agent in stable after promotion (installed as "+installUser+")")
}

// TestIntegrationPreservationRootInstall verifies that an integration initially installed
// by root survives a stable→pipeline-experiment→promote cycle with dd-agent ownership
// restored. This is the adversarial ownership case: pre-upgrade files are root-owned,
// so post.py must chown them (or run pip as dd-agent) to avoid blocking future installs.
func (s *preservationSuite) TestIntegrationPreservationRootInstall() {
	s.runIntegrationOwnershipTest("root", "RootInstall")
}

// TestIntegrationPreservationDDAgentInstall verifies that an integration initially installed
// by dd-agent retains dd-agent ownership after a stable→pipeline-experiment→promote cycle.
// This is the baseline / regression-guard case for the standard install path.
func (s *preservationSuite) TestIntegrationPreservationDDAgentInstall() {
	s.runIntegrationOwnershipTest("dd-agent", "DDAgentInstall")
}

// TestIntegrationPreservationOnExperimentRollback verifies that a third-party integration
// installed on the released OCI stable survives an experiment that gets stopped and rolled
// back. Mirrors TestUpgradeFailureHealth but installs the integration first and asserts it
// is still present on the (now reverted) stable after rollback. Locks in that
// preStartExperiment must not destructively touch the stable's site-packages when only
// saving for the experiment.
func (s *preservationSuite) TestIntegrationPreservationOnExperimentRollback() {
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

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved while experiment is running")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed while experiment is running")
	}, 60*time.Second, 5*time.Second)

	err = s.Backend.StopExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("Rollback: after StopExperiment (rollback triggered)")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		version, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, originalVersion, version)
	}, 300*time.Second, 30*time.Second)
	s.snapshotIntegrationState("Rollback: after rollback completed")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should still be installed on the (reverted) stable after rollback")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after rollback")
	}, 60*time.Second, 5*time.Second)
}

// TestIntegrationPreservationMultiHop verifies that a third-party integration installed
// on the initial released stable survives two consecutive experiment-and-promote cycles
// to different targets. This validates that .post_python_installed_packages.txt is
// correctly refreshed after each promote so the next experiment's diff is computed
// against the right baseline. A broken refresh would either drop the integration from
// the diff (silent loss) or carry stale entries that the new agent can't reinstall.
func (s *preservationSuite) TestIntegrationPreservationMultiHop() {
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

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should be preserved after hop 1 promote")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after hop 1 promote")
	}, 60*time.Second, 5*time.Second)

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

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "integration should still be preserved after hop 2 promote")
		_, showErr := s.Agent.IntegrationShow("datadog-ping")
		assert.NoError(c, showErr, "integration show should succeed after hop 2 promote")
	}, 60*time.Second, 5*time.Second)
}

// TestIntegrationPreservationMixedCustomization verifies that two third-party
// integrations installed on the released stable both survive an experiment-and-promote
// to the pipeline testing version. This exercises the multi-package-diff branch of
// create_diff_installed_packages_file: a single .diff_python_installed_packages.txt
// containing multiple entries, all of which must be reinstalled by post.py.
func (s *preservationSuite) TestIntegrationPreservationMixedCustomization() {
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

	// post.py reinstalls both integrations asynchronously after the daemon restarts;
	// poll until it finishes rather than asserting synchronously.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "ping should be preserved in experiment")
		assert.Equal(c, "2.0.0", intgs["puma"], "puma should be preserved in experiment")
	}, 60*time.Second, 5*time.Second)

	err = s.Backend.PromoteExperiment("datadog-agent")
	s.Require().NoError(err)
	s.snapshotIntegrationState("MixedCustomization: after PromoteExperiment")

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		packageVersion, err := s.Agent.PackageVersion()
		require.NoError(c, err)
		require.Equal(c, testingVersion, packageVersion)
	}, 300*time.Second, 30*time.Second)

	// Same race after promotion: poll until post.py has reinstalled both integrations.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		intgs, err := s.Agent.InstalledIntegrations()
		require.NoError(c, err)
		assert.Equal(c, "1.0.2", intgs["ping"], "ping should be preserved after promotion")
		assert.Equal(c, "2.0.0", intgs["puma"], "puma should be preserved after promotion")
	}, 60*time.Second, 5*time.Second)
	_, showErr := s.Agent.IntegrationShow("datadog-ping")
	s.Assert().NoError(showErr, "integration show for ping should succeed after promotion")
	_, showErr = s.Agent.IntegrationShow("datadog-puma")
	s.Assert().NoError(showErr, "integration show for puma should succeed after promotion")
}
