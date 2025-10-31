// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package injecttests contains the E2E tests for the APM Inject package.
package injecttests

import (
	"time"

	"github.com/cenkalti/backoff"

	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
)

type baseSuite struct {
	installerwindows.BaseSuite
}

func (s *baseSuite) assertSuccessfulPromoteExperiment(version string) {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-inject")
	// verify the driver is running by checking the service status
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", backoff.NewConstantBackOff(30*time.Second), "ddinjector"))
}

func (s *baseSuite) assertDriverInjections() {
	// to check we set DD_INJECT_LOG_SINKS, DD_INJECT_LOG_LEVEL and run any application
	// we should then see logs like this:
	//INFO>1761855931 unknown dd_inject[8292]: [C:\Users\Administrator\auto_inject\src\windows\java.c:89] Instrumenting Java
	//INFO>1761855932 unknown dd_inject[8292]: [C:\Users\Administrator\auto_inject\src\windows\java.c:133] Java instrumentation completed
	//INFO>1761855932 unknown dd_inject[8292]: [C:\Users\Administrator\auto_inject\src\windows\dotnet.c:12] Instrumenting .NET
	//INFO>1761855932 unknown dd_inject[8292]: [C:\Users\Administrator\auto_inject\src\windows\dotnet.c:91] .NET tracing enabled
	// TODO write this function as injection seems to be causing issues

}
