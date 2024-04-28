// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageInstallerSuite struct {
	packageBaseSuite
}

func testInstaller(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageInstallerSuite{
		packageBaseSuite: newPackageSuite("installer", os, arch),
	}
}

func (s *packageInstallerSuite) TestBootstrap() {
	initialState := s.host.State()
	s.Bootstrap()
	defer s.Purge()

	diff := s.host.Diff(initialState)
	diff.AssertGroupAdded("dd-agent")
	diff.AssertUserAdded("dd-agent", "dd-agent")
	diff.AssertDirCreated("/var/log/datadog", 0755, "dd-agent", "dd-agent")
	diff.AssertDirCreated("/var/run/datadog-packages", 0777, "root", "root")
	diff.AssertFileCreated("/usr/bin/datadog-installer", 0777, "root", "root")

	diff.AssertNoOtherChanges()
}
