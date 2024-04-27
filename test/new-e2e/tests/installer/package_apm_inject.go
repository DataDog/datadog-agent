// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageAPMInjectSuite struct {
	packageBaseSuite
}

func testAPMInject(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageAPMInjectSuite{
		packageBaseSuite: newPackageSuite("apm-inject", os, arch),
	}
}

func (s *packageAPMInjectSuite) TestDumpTestSuccess() {
	output := s.Env().RemoteHost.MustExecute("echo 1")
	s.Equal("1", output)
}

func (s *packageAPMInjectSuite) TestDumpTestFailure() {
	output := s.Env().RemoteHost.MustExecute("echo 1")
	s.Equal("2", output)
}
