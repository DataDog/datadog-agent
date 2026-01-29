// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package injecttests contains the E2E tests for the APM Inject package.
package injecttests

import (
	"os"
	"time"

	"github.com/cenkalti/backoff/v5"

	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
)

type baseSuite struct {
	installerwindows.BaseSuite
	currentAPMInjectVersion  installerwindows.PackageVersion
	previousAPMInjectVersion installerwindows.PackageVersion
}

func (s *baseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.currentAPMInjectVersion = installerwindows.NewVersionFromPackageVersion(os.Getenv("CURRENT_APM_INJECT_VERSION"))
	if s.currentAPMInjectVersion.PackageVersion() == "" {
		s.currentAPMInjectVersion = installerwindows.NewVersionFromPackageVersion("0.52.0-dev.b282e14.glci1291404213.g7ff18a26-1")
	}
	s.previousAPMInjectVersion = installerwindows.NewVersionFromPackageVersion(os.Getenv("PREVIOUS_APM_INJECT_VERSION"))
	if s.previousAPMInjectVersion.PackageVersion() == "" {
		s.previousAPMInjectVersion = installerwindows.NewVersionFromPackageVersion("0.50.0-dev.ba30ecb.glci1208428525.g594e53fe-1")
	}
}
func (s *baseSuite) assertSuccessfulPromoteExperiment() {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-inject")
	// verify the driver is running by checking the service status
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

func (s *baseSuite) assertDriverInjections(enabled bool) {
	script := `
# We copy whoami.exe to another directory because System32 is ignored by the driver
$dst = "$env:TEMP\where.exe"
Copy-Item "C:\Windows\System32\whoami.exe" $dst -Force

$env:DD_INJECT_LOG_SINKS = "stdout"
$env:DD_INJECT_LOG_LEVEL = "debug"

& $dst
`
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoError(err)
	if enabled {
		s.Require().Contains(output, "main executable path")
	} else {
		s.Require().NotContains(output, "main executable path")
	}
}
