// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package injecttests contains the E2E tests for the APM Inject package.
package injecttests

import (
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
)

type baseSuite struct {
	installerwindows.BaseSuite
}

func (s *baseSuite) assertSuccessfulPromoteExperiment(version string) {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-inject").
		WithStableVersionMatchPredicate(func(actual string) {
			s.Require().Contains(actual, version)
		}).
		WithExperimentVersionEqual("")
}
