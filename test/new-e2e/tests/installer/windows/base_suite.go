// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type baseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer *datadogInstaller
}

func (s *baseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// TODO:FA-779
	if s.Env().AwsEnvironment.PipelineID() == "" {
		s.T().Logf("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
		s.T().FailNow()
	}
	s.installer = NewDatadogInstaller(s.Env())
}

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog Installer `baseSuite` object.
func (s *baseSuite) Require() *SuiteAssertions {
	return &SuiteAssertions{
		Assertions: s.BaseSuite.Require(),
		testing:    s.T(),
		env:        s.Env(),
	}
}
