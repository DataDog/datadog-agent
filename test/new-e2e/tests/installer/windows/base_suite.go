// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
)

type baseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer *DatadogInstaller
}

func (s *baseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// TODO:FA-779
	if s.Env().AwsEnvironment.PipelineID() == "" {
		s.FailNow("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}
}

func (s *baseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	outputDir, err := runner.GetTestOutputDir(runner.GetProfile(), s.T())
	s.Require().NoError(err, "should get output dir")
	s.T().Logf("Output dir: %s", outputDir)
	s.installer = NewDatadogInstaller(s.Env(), fmt.Sprintf("%s/install.log", outputDir))
}

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog Installer `baseSuite` object.
func (s *baseSuite) Require() *suiteasserts.SuiteAssertions {
	return suiteasserts.New(s.BaseSuite.Require(), s)
}
