// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
)

type packageInstallerSuite struct {
	packageBaseSuite
}

func testInstaller(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageInstallerSuite{
		packageBaseSuite: newPackageSuite("installer", os, arch, awshost.WithoutFakeIntake()),
	}
}

func (s *packageInstallerSuite) TestInstall() {
	s.RunInstallScript()
	defer s.Purge()

	bootstraperVersion := s.BootstraperVersion()
	installerVersion := s.InstallerVersion()
	assert.Equal(s.T(), bootstraperVersion, installerVersion)

	state := s.host.State()
	state.AssertGroupExists("dd-agent")
	state.AssertUserExists("dd-agent")
	state.AssertUserHasGroup("dd-agent", "dd-agent")

	state.AssertDirExists("/var/log/datadog", 0755, "dd-agent", "dd-agent")
	state.AssertDirExists("/var/run/datadog-packages", 0777, "root", "root")

	state.AssertDirExists("/opt/datadog-installer", 0755, "root", "root")
	state.AssertDirExists("/opt/datadog-packages", 0755, "root", "root")
	state.AssertDirExists("/opt/datadog-packages/datadog-installer", 0755, "root", "root")
	state.AssertDirExists("/opt/datadog-packages/datadog-installer/stable/run", 0755, "dd-agent", "dd-agent")

	state.AssertSymlinkExists("/usr/bin/datadog-bootstrap", "/opt/datadog-installer/bin/installer/installer", "root", "root")
	state.AssertSymlinkExists("/usr/bin/datadog-installer", "/opt/datadog-packages/datadog-installer/stable/bin/installer/installer", "root", "root")

	state.AssertUnitsNotLoaded("datadog-installer.service", "datadog-installer-exp.service")
}

func (s *packageInstallerSuite) TestUninstall() {
	s.RunInstallScript()
	s.Purge()

	state := s.host.State()

	// state that never should get removed
	state.AssertGroupExists("dd-agent")
	state.AssertUserExists("dd-agent")
	state.AssertUserHasGroup("dd-agent", "dd-agent")

	state.AssertDirExists("/var/log/datadog", 0755, "dd-agent", "dd-agent")

	// state that should get removed
	state.AssertPathDoesNotExist("/opt/datadog-installer")
	state.AssertPathDoesNotExist("/var/run/datadog-packages")
	state.AssertPathDoesNotExist("/opt/datadog-packages")

	state.AssertPathDoesNotExist("/usr/bin/datadog-bootstrap")
	state.AssertPathDoesNotExist("/usr/bin/datadog-installer")
}
