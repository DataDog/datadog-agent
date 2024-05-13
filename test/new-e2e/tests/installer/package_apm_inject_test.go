// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageApmInjectSuite struct {
	packageBaseSuite
}

func testApmInjectAgent(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageApmInjectSuite{
		packageBaseSuite: newPackageSuite("apm-inject", os, arch),
	}
}

func (s *packageApmInjectSuite) TestInstall() {
	s.RunInstallScript()
	defer s.Purge()
	s.InstallAgentPackage()
	s.InstallPackageLatest("datadog-apm-inject")
	s.InstallPackageLatest("datadog-apm-library-python")

	state := s.host.State()

	state.AssertFileExists("/etc/ld.so.preload", 0644, "root", "root")
}
