// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"flag"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	suite_assertions "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
)

type baseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	installer *datadogInstaller
}

var (
	msiLogPath = flag.String("log-path", "", "the location where to store the installation logs on the local host. By default it will use a temporary folder.")
)

func (s *baseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// TODO:FA-779
	if s.Env().AwsEnvironment.PipelineID() == "" {
		s.FailNow("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
	}
	// If *msiLogPath == "" then we will use a temporary path on the host
	s.installer = NewDatadogInstaller(s.Env(), *msiLogPath)
}

// Require instantiates a suiteAssertions for the current suite.
// This allows writing assertions in a "natural" way, i.e.:
//
//	suite.Require().HasAService(...).WithUserSid(...)
//
// Ideally this suite assertion would exist at a higher level of abstraction
// so that it could be shared by multiple suites, but for now it exists only
// on the Windows Datadog Installer `baseSuite` object.
func (s *baseSuite) Require() *suite_assertions.SuiteAssertions {
	return suite_assertions.New(s.BaseSuite.Require(), s)
}
