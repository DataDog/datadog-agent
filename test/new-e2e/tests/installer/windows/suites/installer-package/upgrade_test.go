// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installertests

import (
	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
	"testing"
)

type testInstallerUpgradesSuite struct {
	installerwindows.BaseInstallerSuite
}

// TestInstallerUpgrades tests the upgrades of the Datadog installer on a system.
func TestInstallerUpgrades(t *testing.T) {
	e2e.Run(t, &testInstallerUpgradesSuite{}, e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
}

// TestUpgrades tests upgrading the stable version of the Datadog installer to the latest from the pipeline.
func (s *testInstallerUpgradesSuite) TestUpgrades() {
	// Arrange
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithInstallerURLFromInstallersJSON(pipeline.AgentS3BucketTesting, pipeline.StableChannel, s.StableInstallerVersion().PackageVersion())),
		installerwindows.WithMSILogFile("install.log"),
	)
	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(installerwindows.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionEqual(s.StableInstallerVersion().Version())

	// Act
	// Install "latest" from the pipeline
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithMSILogFile("upgrade.log"),
	))

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(installerwindows.BinaryPath).
		WithSignature(agent.GetCodeSignatureThumbprints()).
		WithVersionMatchPredicate(func(version string) {
			// We have to use a predicate and parse the installer's version here because unlike the stable format
			// we only have a current "Agent" version, which uses a different format than the installer's.
			// For example in a CI pipeline:
			//		CURRENT_AGENT_VERSION: 7.57.0-devel+git.479.c6f7923.pipeline.40641070
			//		version: 7.57.0-devel+git.481.634b7cd
			actualVersion, err := agentVersion.New(version, "")
			s.Require().NoError(err, "Agent version was in the wrong format")
			s.Require().Equal(s.CurrentAgentVersion().GetNumberAndPre(), actualVersion.GetNumberAndPre())
		})
}
