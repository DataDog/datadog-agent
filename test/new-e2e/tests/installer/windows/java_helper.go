// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	suiteasserts "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/suite-assertions"
)

// JavaHelper provides reusable Java testing functionality that can be composed with any test suite.
// It requires a suite that implements the WindowsSuite interface to access the remote host and assertions.
type JavaHelper struct {
	suite WindowsSuite
}

// NewJavaHelper creates a new JavaHelper with the given suite.
func NewJavaHelper(s WindowsSuite) *JavaHelper {
	return &JavaHelper{suite: s}
}

// SetupJava installs Java JDK on the remote host.
// This should be called in the suite's SetupSuite method.
func (h *JavaHelper) SetupJava() {
	h.installChocolatey()
	h.installJava()
}

func (h *JavaHelper) installChocolatey() {
	host := h.suite.Env().RemoteHost
	script := `
if (!(Get-Command choco -ErrorAction SilentlyContinue)) {
	Set-ExecutionPolicy Bypass -Scope Process -Force
	[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
	Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
}
	`
	output, err := host.Execute(script)
	h.suite.Require().NoErrorf(err, "failed to install Chocolatey: %s", output)
}

func (h *JavaHelper) installJava() {
	host := h.suite.Env().RemoteHost
	output, err := host.Execute("choco install openjdk11 -y")
	h.suite.Require().NoErrorf(err, "failed to install Java: %s", output)

	// Verify Java installation
	output, err = host.Execute("java -version")
	h.suite.Require().NoErrorf(err, "failed to verify Java installation: %s", output)
}

// StartJavaApp deploys, compiles, and runs a simple Java application.
// Returns the output from running the application to verify injection.
func (h *JavaHelper) StartJavaApp(javaSourceCode []byte) string {
	host := h.suite.Env().RemoteHost

	err := host.MkdirAll("C:\\JavaApp")
	h.suite.Require().NoError(err, "failed to create directory for JavaApp")

	_, err = host.WriteFile("C:\\JavaApp\\DummyApp.java", javaSourceCode)
	h.suite.Require().NoError(err, "failed to write Java source file")

	output, err := host.Execute("cd C:\\JavaApp; javac DummyApp.java")
	h.suite.Require().NoErrorf(err, "failed to compile Java app: %s", output)

	output, err = host.Execute("cd C:\\JavaApp; java DummyApp")
	h.suite.Require().NoErrorf(err, "failed to run Java app: %s", output)

	return strings.TrimSpace(output)
}