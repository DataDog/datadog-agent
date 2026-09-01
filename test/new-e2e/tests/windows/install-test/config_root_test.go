// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"
)

const guestsSid = "S-1-5-32-546"

func TestInstallWithUntrustedConfigRoot(t *testing.T) {
	s := &testUntrustedConfigRootSuite{}
	Run(t, s)
}

type testUntrustedConfigRootSuite struct {
	baseAgentMSISuite
}

// TestInstallWithUntrustedConfigRoot tests that the installer refuses to install into a
// configuration directory that is not owned by Administrators or SYSTEM, and that the remediation
// the error message suggests lets the installation proceed.
func (s *testUntrustedConfigRootSuite) TestInstallWithUntrustedConfigRoot() {
	vm := s.Env().RemoteHost
	configRoot := `C:\ProgramData\Datadog`

	s.Require().NoError(vm.MkdirAll(configRoot))
	// A file the installer must not adopt, to show the directory is left untouched
	plantedConfig := filepath.Join(configRoot, "datadog.yaml")
	_, err := vm.WriteFile(plantedConfig, []byte("api_key: planted\n"))
	s.Require().NoError(err)
	_, err = vm.Execute(fmt.Sprintf(`icacls "%s" /setowner "*%s"`, configRoot, guestsSid))
	s.Require().NoError(err)

	logFile := filepath.Join(s.SessionOutputDir(), "install.log")
	_, err = s.InstallAgent(vm,
		windowsAgent.WithPackage(s.AgentPackage),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithInstallLogFile(logFile),
	)
	s.Require().Error(err, "install should fail when the configuration directory is owned by an unprivileged user")

	s.Run("reports the reason and the remediation", func() {
		contents, err := os.ReadFile(logFile)
		s.Require().NoError(err)
		contents, err = windowsCommon.ConvertUTF16ToUTF8(contents)
		s.Require().NoError(err)
		// Not assert.Contains, it prints the whole log on failure
		s.Assert().True(strings.Contains(string(contents), "has unexpected owner"),
			"the log should report the owner that was found")
		s.Assert().True(strings.Contains(string(contents), "takeown.exe"),
			"the log should report how to resolve it")
	})

	s.Run("installs nothing", func() {
		_, err := vm.Lstat(`C:\Program Files\Datadog\Datadog Agent`)
		s.Assert().ErrorIs(err, fs.ErrNotExist, "should not create the install directory")

		for _, serviceName := range servicetest.ExpectedInstalledServices() {
			_, err := windowsCommon.GetServiceConfig(vm, serviceName)
			s.Assert().Errorf(err, "should not create service %s", serviceName)
		}

		registryKeyExists, err := windowsCommon.RegistryKeyExists(vm, windowsAgent.RegistryKeyPath)
		s.Assert().NoError(err, "should check registry key exists")
		s.Assert().False(registryKeyExists, "should not create the registry key")
	})

	s.Run("leaves the directory untouched", func() {
		contents, err := vm.ReadFile(plantedConfig)
		s.Assert().NoError(err, "should not remove the existing configuration file")
		s.Assert().Contains(string(contents), "api_key: planted", "should not modify the existing configuration file")
	})

	s.Run("installs after taking ownership", func() {
		// The remediation from the error message
		_, err := vm.Execute(fmt.Sprintf(`takeown.exe /A /F "%s"`, configRoot))
		s.Require().NoError(err)
		s.Require().NoError(vm.Remove(plantedConfig))

		s.installAgentPackage(vm, s.AgentPackage)
	})
}
