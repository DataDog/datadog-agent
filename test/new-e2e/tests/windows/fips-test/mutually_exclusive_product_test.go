// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipstest

import (
	"os"
	"path/filepath"
	"strings"

	installtest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/require"
)

type mutuallyExclusiveInstallSuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]

	previousAgentPackage *windowsAgent.Package
}

// TestFIPSAgentDoesNotInstallOverAgent tests that the FIPS agent cannot be installed over the base Agent
//
// This test uses the last stable base Agent package and the pipeline produced FIPS Agent package.
func TestFIPSAgentDoesNotInstallOverAgent(t *testing.T) {
	s := &mutuallyExclusiveInstallSuite{}
	os.Setenv(windowsAgent.PackageFlavorEnvVar, "base")
	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	require.NoError(t, err, "should get last stable agent package from env")
	s.previousAgentPackage = previousAgentPackage
	os.Setenv(windowsAgent.PackageFlavorEnvVar, "fips")
	installtest.Run[environments.WindowsHost](t, s)
}

// TestAgentDoesNotInstallOverFIPSAgent tests that the base Agent cannot be installed over the FIPS agent
//
// This test uses the pipeline produced MSI packages for both flavors. This is necessary for now
// because the previous Agent versions do not contain the changes to detect mutually exclusive products.
func TestAgentDoesNotInstallOverFIPSAgent(t *testing.T) {
	s := &mutuallyExclusiveInstallSuite{}
	os.Setenv(windowsAgent.PackageFlavorEnvVar, "fips")
	previousAgentPackage, err := windowsAgent.GetPackageFromEnv()
	require.NoError(t, err, "should get Agent package from env")
	s.previousAgentPackage = previousAgentPackage
	os.Setenv(windowsAgent.PackageFlavorEnvVar, "base")
	installtest.Run[environments.WindowsHost](t, s)
}

func (s *mutuallyExclusiveInstallSuite) SetupSuite() {
	// Base looks up the first Agent package
	s.BaseAgentInstallerSuite.SetupSuite()
	host := s.Env().RemoteHost
	var err error

	s.T().Logf("Using previous Agent package: %#vvi", s.previousAgentPackage)

	// Install first Agent
	_, err = s.InstallAgent(host, windowsAgent.WithPackage(s.previousAgentPackage))
	s.Require().NoError(err)
}

func (s *mutuallyExclusiveInstallSuite) TestMutuallyExclusivePackage() {
	host := s.Env().RemoteHost

	// Install second Agent
	logFilePath := filepath.Join(s.SessionOutputDir(), "secondInstall.log")
	_, err := s.InstallAgent(host,
		windowsAgent.WithPackage(s.AgentPackage),
		windowsAgent.WithInstallLogFile(logFilePath),
	)
	s.Require().Error(err)

	// Ensure that the log file contains the expected error message
	logData, err := os.ReadFile(logFilePath)
	s.Require().NoError(err)
	// convert from utf-16 to utf-8
	logData, err = windowsCommon.ConvertUTF16ToUTF8(logData)
	s.Require().NoError(err)
	// We don't use assert.Contains because it will print the very large logData on error
	s.Assert().True(strings.Contains(string(logData), "This product cannot be installed at the same time as "))
}
