// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

type baseIISSuite struct {
	BaseSuite
	iisHelper *IISHelper
}

func (s *baseIISSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.iisHelper = NewIISHelper(s)
	s.iisHelper.SetupIIS()
}

func (s *baseIISSuite) startIISApp(webConfigFile, aspxFile []byte) {
	s.iisHelper.StartIISApp(webConfigFile, aspxFile)
}

func (s *baseIISSuite) stopIISApp() {
	s.iisHelper.StopIISApp()
}

func (s *baseIISSuite) getLibraryPathFromInstrumentedIIS() string {
	return s.iisHelper.GetLibraryPathFromInstrumentedIIS()
}

func (s *baseIISSuite) assertSuccessfulPromoteExperiment(version string) {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-library-dotnet").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("")
}
