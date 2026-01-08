// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"strings"
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
	host := h.suite.Env().RemoteHost

	script := `
# Install Chocolatey if not already installed
if (!(Get-Command choco -ErrorAction SilentlyContinue)) {
	Set-ExecutionPolicy Bypass -Scope Process -Force
	[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
	Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

	# Refresh PATH for current session
	$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}

# Install Java
choco install openjdk11 -y

# Refresh PATH again to pick up Java
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# Verify Java installation
java -version
	`
	output, err := host.Execute(script)
	h.suite.Require().NoErrorf(err, "failed to install Java: %s", output)
	// force host to reconnect to update the PATH
	err = host.Reconnect()
	h.suite.Require().NoError(err, "failed to reconnect to host")
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

	script := `
$env:DD_INJECT_LOG_SINKS = "stdout"
$env:DD_INJECT_LOG_LEVEL = "debug"
cd C:\JavaApp
java DummyApp
	`
	output, err = host.Execute(script)
	h.suite.Require().NoErrorf(err, "failed to run Java app: %s", output)

	return strings.TrimSpace(output)
}
