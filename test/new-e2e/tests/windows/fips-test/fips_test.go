// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fipstest
package fipstest

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fipsAgentSuite struct {
	windows.BaseAgentInstallerSuite[environments.WindowsHost]

	installPath string
}

func TestFIPSAgent(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake())}
	s := &fipsAgentSuite{}
	e2e.Run(t, s, opts...)
}

func (s *fipsAgentSuite) SetupSuite() {
	// Default to using FIPS Agent package
	if _, set := windowsAgent.LookupFlavorFromEnv(); !set {
		os.Setenv(windowsAgent.PackageFlavorEnvVar, "fips")
	}

	s.BaseAgentInstallerSuite.SetupSuite()
	host := s.Env().RemoteHost
	var err error

	// Enable FIPS mode before installing the Agent to make sure that works
	err = windowsCommon.EnableFIPSMode(host)
	require.NoError(s.T(), err)

	// Install Agent (With FIPS mode enabled)
	_, err = s.InstallAgent(host, windowsAgent.WithPackage(s.AgentPackage))
	require.NoError(s.T(), err)

	s.installPath, err = windowsAgent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
}

func (s *fipsAgentSuite) TestWithSystemFIPSDisabled() {
	host := s.Env().RemoteHost
	windowsCommon.DisableFIPSMode(host)

	s.Run("version command", func() {
		s.Run("gofips enabled", func() {
			_, err := s.execAgentCommandWithFIPS("version")
			assertErrorContainsFIPSPanic(s.T(), err, "agent should panic when GOFIPS=1 but system FIPS is disabled")
		})

		s.Run("gofips disabled", func() {
			_, err := s.execAgentCommand("version")
			require.NoError(s.T(), err)
		})
	})
}

func (s *fipsAgentSuite) TestWithSystemFIPSEnabled() {
	host := s.Env().RemoteHost
	windowsCommon.EnableFIPSMode(host)

	s.Run("version command", func() {
		s.Run("gofips enabled", func() {
			_, err := s.execAgentCommandWithFIPS("version")
			require.NoError(s.T(), err)
		})

		s.Run("gofips disabled", func() {
			_, err := s.execAgentCommand("version")
			require.NoError(s.T(), err)
		})
	})
}

func (s *fipsAgentSuite) TestFIPSProviderPresent() {
	host := s.Env().RemoteHost
	exists, _ := host.FileExists(path.Join(s.installPath, "embedded3/lib/ossl-modules/fips.dll"))
	require.True(s.T(), exists, "Agent install path should contain the FIPS provider but doesn't")
}

func (s *fipsAgentSuite) TestFIPSInstall() {
	host := s.Env().RemoteHost
	openssl := path.Join(s.installPath, "embedded3/bin/openssl.exe")
	cmd := fmt.Sprintf("%s fipsinstall", openssl)
	_, err := host.Execute(cmd)
	require.NoError(s.T(), err)
}

func (s *fipsAgentSuite) execAgentCommand(command string, options ...client.ExecuteOption) (string, error) {
	host := s.Env().RemoteHost

	require.NotEmpty(s.T(), s.installPath)
	agentPath := filepath.Join(s.installPath, "bin", "agent.exe")

	cmd := fmt.Sprintf(`& "%s" %s`, agentPath, command)
	return host.Execute(cmd, options...)
}

func (s *fipsAgentSuite) execAgentCommandWithFIPS(command string) (string, error) {
	// There isn't support for appending env vars to client.ExecuteOption, so
	// this function doesn't accept any other options.

	// Setting GOFIPS=1 causes the Windows FIPS Agent to panic if the system is not in FIPS mode.
	// This setting does NOT control whether the FIPS Agent uses FIPS-compliant crypto libraries,
	// the System-level setting determines that.
	// https://github.com/microsoft/go/tree/microsoft/main/eng/doc/fips#windows-fips-mode-cng
	vars := client.EnvVar{
		"GOFIPS": "1",
	}

	return s.execAgentCommand(command, client.WithEnvVariables(vars))
}

func assertErrorContainsFIPSPanic(t *testing.T, err error, args ...interface{}) bool {
	return assert.ErrorContains(t, err, "panic: cngcrypto: not in FIPS mode", args...)
}
