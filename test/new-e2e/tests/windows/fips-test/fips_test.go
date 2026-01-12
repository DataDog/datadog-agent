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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
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
	configRoot  string
}

func TestFIPSAgent(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake())}
	s := &fipsAgentSuite{}
	e2e.Run(t, s, opts...)
}

func TestFIPSAgentAltDir(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake())}
	s := &fipsAgentSuite{
		installPath: "C:\\altdir",
		configRoot:  "C:\\altconfroot",
	}
	e2e.Run(t, s, opts...)
}

func (s *fipsAgentSuite) SetupSuite() {
	// Default to using FIPS Agent package
	if _, set := windowsAgent.LookupFlavorFromEnv(); !set {
		os.Setenv(windowsAgent.PackageFlavorEnvVar, "fips")
	}

	s.BaseAgentInstallerSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost
	var err error

	// Enable FIPS mode before installing the Agent to make sure that works
	err = windowsCommon.EnableFIPSMode(host)
	require.NoError(s.T(), err)

	// Install Agent (With FIPS mode enabled)
	opts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(s.AgentPackage),
	}
	if s.installPath != "" {
		opts = append(opts, windowsAgent.WithProjectLocation(s.installPath))
	}
	if s.configRoot != "" {
		opts = append(opts, windowsAgent.WithApplicationDataDirectory(s.configRoot))
	}
	_, err = s.InstallAgent(host, opts...)
	require.NoError(s.T(), err)

	s.installPath, err = windowsAgent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)
	s.configRoot, err = windowsAgent.GetConfigRootFromRegistry(host)
	require.NoError(s.T(), err)
}

func (s *fipsAgentSuite) TestFIPSProviderPresent() {
	host := s.Env().RemoteHost
	exists, _ := host.FileExists(path.Join(s.installPath, "embedded3/lib/ossl-modules/fips.dll"))
	require.True(s.T(), exists, "Agent install path should contain the FIPS provider but doesn't")
}

// TestFIPSInstall tests that the MSI created a valid fipsmodule.cnf
func (s *fipsAgentSuite) TestFIPSInstall() {
	host := s.Env().RemoteHost
	openssl := path.Join(s.installPath, "embedded3/bin/openssl.exe")
	fipsModule := path.Join(s.installPath, "embedded3/lib/ossl-modules/fips.dll")
	fipsConf := path.Join(s.installPath, "embedded3/ssl/fipsmodule.cnf")
	cmd := fmt.Sprintf(`& "%s" fipsinstall -module "%s" -in "%s" -verify`, openssl, fipsModule, fipsConf)
	_, err := host.Execute(cmd)
	require.NoError(s.T(), err, "MSI should create valid fipsmodule.cnf")
}

// TestOpenSSLPaths tests that the MSI sets the OpenSSL paths in the registry
func (s *fipsAgentSuite) TestOpenSSLPaths() {
	host := s.Env().RemoteHost

	// assert openssl winctx registry keys exist
	// https://github.com/openssl/openssl/blob/master/NOTES-WINDOWS.md#installation-directories
	expectedOpenSSLPaths := map[string]string{
		"OPENSSLDIR": s.installPath + "embedded3\\ssl",
		"ENGINESDIR": s.installPath + "embedded3\\lib\\engines-3",
		"MODULESDIR": s.installPath + "embedded3\\lib\\ossl-modules",
	}
	// TODO: How to configure the version of OpenSSL?
	opensslVersion := "3.5"
	keyPath := fmt.Sprintf(`HKLM:\SOFTWARE\Wow6432Node\OpenSSL-%s-datadog-fips-agent`, opensslVersion)
	exists, err := windowsCommon.RegistryKeyExists(host, keyPath)
	require.NoError(s.T(), err)
	if assert.True(s.T(), exists, "%s should exist", keyPath) {
		for name, expected := range expectedOpenSSLPaths {
			// check value matches
			value, err := windowsCommon.GetRegistryValue(host, keyPath, name)
			if assert.NoError(s.T(), err, "Failed to get %s", name) {
				assert.Equal(s.T(), expected, value, "Unexpected value for %s", name)
			}
			// ensure value exists as a directory
			fileInfo, err := host.Lstat(value)
			if assert.NoError(s.T(), err, "Path %s for %s does not exist", value, name) {
				assert.True(s.T(), fileInfo.IsDir(), "Path %s for %s is not a directory", value, name)
			}
		}
	}

	// assert that openssl uses the paths from the registry
	// Example output:
	// 	OpenSSL 3.3.2 3 Sep 2024 (Library: OpenSSL 3.3.2 3 Sep 2024)
	//  <snipped>
	//  compiler: gcc <snipped> -DOSSL_WINCTX=datadog-fips-agent
	//  OPENSSLDIR: "C:\Program Files\Datadog\Datadog Agent\embedded3\ssl"
	//  ENGINESDIR: "C:\Program Files\Datadog\Datadog Agent\embedded3\lib\engines-3"
	//  MODULESDIR: "C:\Program Files\Datadog\Datadog Agent\embedded3\lib\ossl-modules"
	openssl := path.Join(s.installPath, `embedded3\bin\openssl.exe`)
	cmd := fmt.Sprintf(`& "%s" version -a`, openssl)
	out, err := host.Execute(cmd)
	require.NoError(s.T(), err)
	assert.Contains(s.T(), out, `-DOSSL_WINCTX=datadog-fips-agent`, "Expected -DOSSL_WINCTX=datadog-fips-agent in openssl.exe output")
	for name, expected := range expectedOpenSSLPaths {
		assert.Contains(s.T(), out, fmt.Sprintf(`%s: "%s"`, name, expected), "Expected %s to be %s", name, expected)
	}
}
