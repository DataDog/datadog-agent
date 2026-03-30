// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	unixinstaller "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
)

type testAgentGCSuite struct {
	installerwindows.BaseSuite
}

// TestAgentGC tests the garbage collection behavior of the Datadog Agent package.
func TestAgentGC(t *testing.T) {
	e2e.Run(t, &testAgentGCSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestGarbageCollectWithStableAsRealDirectory is a regression test for a bug where hosts
// whose "stable" entry in the packages directory was a real directory instead of a symlink
// (e.g. after a failed or legacy Windows installation) would fail GarbageCollect on every
// GC cycle, blocking further upgrades indefinitely.
//
// The fix makes newLink() treat a non-symlink "stable" entry as a missing link so that
// cleanup can proceed without returning an error.
func (s *testAgentGCSuite) TestGarbageCollectWithStableAsRealDirectory() {
	// Arrange: install the agent so the packages directory and daemon are set up.
	host := s.Env().RemoteHost
	host.MkdirAll(`C:\ProgramData\Datadog`)
	host.WriteFile(`C:\ProgramData\Datadog\datadog.yaml`, []byte(`
api_key: `+unixinstaller.GetAPIKey()+`
site: datadoghq.com
remote_updates: true
log_level: debug
`))
	err := s.Installer().Install(
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-current-version.log"),
	)
	s.Require().NoError(err, "agent installation should succeed")
	s.Require().NoError(s.WaitForInstallerService("Running"))

	stablePath := consts.GetStableDirFor(consts.AgentPackage)

	// Corrupt the state: replace the "stable" symlink with a real directory.
	// Remove-Item without -Recurse removes the symlink itself, not its target contents.
	_, err = host.Execute(fmt.Sprintf(
		`Remove-Item -Path "%s" -Force; New-Item -ItemType Directory -Path "%s" | Out-Null`,
		stablePath, stablePath,
	))
	s.Require().NoError(err, "should replace stable symlink with a real directory")

	// Sanity-check: LinkType is empty for a real directory, "SymbolicLink" for a symlink.
	linkType, err := host.Execute(fmt.Sprintf(`(Get-Item -Path "%s").LinkType`, stablePath))
	s.Require().NoError(err)
	s.Require().Empty(strings.TrimSpace(linkType),
		"stable should now be a real directory (empty LinkType), not a symlink")

	// Act: garbage collection must succeed despite the corrupted state.
	output, err := s.Installer().GarbageCollect()
	s.Require().NoErrorf(err,
		"GarbageCollect should succeed when stable is a real directory, got: %s", output)

	// Assert: the stable directory is preserved — cleanup must not delete it.
	s.Require().Host(host).DirExists(stablePath,
		"stable directory should still exist after GarbageCollect")
}
