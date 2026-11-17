// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/backend"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"
)

const (
	// Keep in sync with the upgrade-history CI minor-offset matrix.
	upgradeHistoryMinorCount     = 4
	upgradeHistoryMinorOffsetEnv = "E2E_FLEET_UPGRADE_MINOR_OFFSET"
)

type upgradeHistorySuite struct {
	suite.FleetSuite
	sourceVersion string
	targetVersion string
}

// TestFleetUpgradeHistory upgrades every released patch/package revision in the
// historical window, with a fresh VM for each source/platform pair.
func TestFleetUpgradeHistory(t *testing.T) {
	platforms := suite.Platforms()
	if len(platforms) == 0 {
		t.Skip("no Fleet platforms selected")
	}
	first, last := 1, upgradeHistoryMinorCount
	if value := os.Getenv(upgradeHistoryMinorOffsetEnv); value != "" {
		offset, err := strconv.Atoi(value)
		require.NoError(t, err, "%s must be an integer from 1 to %d", upgradeHistoryMinorOffsetEnv, upgradeHistoryMinorCount)
		require.GreaterOrEqual(t, offset, 1)
		require.LessOrEqual(t, offset, upgradeHistoryMinorCount)
		first, last = offset, offset
	}

	catalog := backend.LoadUpgradeHistoryCatalog(t)
	target := catalog.Latest(backend.BranchTesting, "datadog-agent")
	for offset := first; offset <= last; offset++ {
		t.Run(fmt.Sprintf("N-%d", offset), func(t *testing.T) {
			versions, err := catalog.PreviousMinorVersions(backend.BranchStable, "datadog-agent", target, offset)
			if errors.Is(err, backend.ErrNoReleasedVersions) {
				// main can advance to N before N-1 has its first stable release.
				// Keep the empty minor visible; never substitute an older minor.
				t.Skip(err.Error())
			}
			require.NoError(t, err)
			t.Logf("Historical upgrade sources: %v; pipeline target: %s", versions, target)
			for _, source := range versions {
				t.Run(source, func(t *testing.T) {
					t.Parallel()
					suite.Run(t, func() e2e.Suite[environments.Host] {
						return &upgradeHistorySuite{sourceVersion: source, targetVersion: target}
					}, platforms, awshost.WithRunOptions(ec2.WithoutFakeIntake()))
				})
			}
		})
	}
}

func (s *upgradeHistorySuite) TestUpgrade() {
	s.T().Logf("Upgrading %s directly to %s", s.sourceVersion, s.targetVersion)
	s.Agent.MustInstall(agent.WithRemoteUpdates(), agent.WithStablePackageVersion(s.sourceVersion))
	defer s.Agent.MustUninstall()

	assertPackageVersion := func(expected string) {
		s.T().Helper()
		require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
			version, err := s.Agent.PackageVersion()
			require.NoError(c, err)
			require.Equal(c, strings.TrimPrefix(expected, "v"), version)
		}, 5*time.Minute, 10*time.Second)
	}
	sourceVersion := strings.TrimPrefix(s.sourceVersion, "v")
	sourceAgentVersion, _, _ := strings.Cut(sourceVersion, "-")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		installedVersion, err := s.Agent.NativePackageVersion()
		require.NoError(c, err)
		require.Equal(c, sourceVersion, installedVersion, "historical package revision must be installed before upgrading")
		runningVersion, err := s.Agent.Version()
		require.NoError(c, err)
		require.Equal(c, sourceAgentVersion, runningVersion, "the historical Agent must be running before upgrading")
	}, 5*time.Minute, 10*time.Second)
	s.Require().NoError(s.Backend.StartExperiment("datadog-agent", s.targetVersion))
	assertPackageVersion(s.targetVersion)
	s.Require().NoError(s.Backend.PromoteExperiment("datadog-agent"))
	assertPackageVersion(s.targetVersion)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		state, err := s.Backend.RemoteConfigStatusPackage("datadog-agent")
		require.NoError(c, err)
		require.Equal(c, s.targetVersion, state.StableVersion)
		require.Empty(c, state.ExperimentVersion, "the candidate must be promoted, not left in an experiment")
	}, 5*time.Minute, 10*time.Second)
}
