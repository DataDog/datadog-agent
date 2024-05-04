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
	s.RunInstallScript()
}
