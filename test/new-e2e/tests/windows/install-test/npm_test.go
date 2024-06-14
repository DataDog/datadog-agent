// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestNPMUpgradeToNPM tests the latest installer can successfully upgrade
// from a version when NPM was optional and not installed.
//
// Old name: Scenario 1
func TestNPMUpgradeToNPM(t *testing.T) {
	s := &testNPMUpgradeToNPMSuite{}
	s.previousVersion = "7.42.0-1"
	run(t, s)
}

type testNPMUpgradeToNPMSuite struct {
	testNPMInstallSuite
}

func (s *testNPMUpgradeToNPMSuite) TestNPMUgpradeToNPM() {
	// s.installPreviousAgentVersion(s.Env().RemoteHost)
	// s.Require().False(s.isNPMInstalled(), "NPM should not be installed")

	// // upgrade to the new version
	// s.upgradeAgent(s.Env().RemoteHost, s.AgentPackage)

	// run tests
	// s.Require().True(s.isNPMInstalled(), "NPM should be installed")
	// s.enableNPM()
	s.testNPMFunctional()
	s.T().FailNow()
}

// TestNPMUpgradeNPMToNPM tests the latest installer can successfully upgrade
// from a version when NPM was optional and was installed.
//
// Old name: Scenario 3
func TestNPMUpgradeNPMToNPM(t *testing.T) {
	s := &testNPMUpgradeNPMToNPMSuite{}
	s.previousVersion = "7.42.0-1"
	run(t, s)
}

type testNPMUpgradeNPMToNPMSuite struct {
	testNPMInstallSuite
}

func (s *testNPMUpgradeNPMToNPMSuite) TestNPMUpgradeNPMToNPM() {
	// Install previous version with ADDLOCAL=NPM
	s.installPreviousAgentVersion(s.Env().RemoteHost,
		windowsAgent.WithAddLocal("NPM"),
	)
	s.Require().True(s.isNPMInstalled(), "NPM should be installed")

	// upgrade to the new version
	s.upgradeAgent(s.Env().RemoteHost, s.AgentPackage)

	// run tests
	s.Require().True(s.isNPMInstalled(), "NPM should be installed")
	s.enableNPM()
	s.testNPMFunctional()
}

// TestNPMInstallWithAddLocal tests the latest installer can successfully install
// when the legacy ADDLOCAL=NPM option is provided
//
// Old name: Scenario 9
func TestNPMInstallWithAddLocal(t *testing.T) {
	s := &testNPMInstallWithAddLocalSuite{}
	run(t, s)
}

type testNPMInstallWithAddLocalSuite struct {
	testNPMInstallSuite
}

func (s *testNPMInstallWithAddLocalSuite) TestNPMInstallWithAddLocal() {
	// install latest with ADDLOCAL=NPM
	_ = s.installAgentPackage(s.Env().RemoteHost,
		s.AgentPackage,
		windowsAgent.WithAddLocal("NPM"),
	)

	// run tests
	s.Require().True(s.isNPMInstalled(), "NPM should be installed")
	s.enableNPM()
	s.testNPMFunctional()
}

// TestNPMUpgradeNPMToNPM tests the latest installer can successfully upgrade
// from a the original NPM beta version.
//
// Old name: Scenario 10
func TestNPMUpgradeFromBeta(t *testing.T) {
	s := &testNPMUpgradeFromBeta{}
	s.previousVersion = "7.23.2-beta1-1"
	s.url = "https://ddagent-windows-unstable.s3.amazonaws.com/datadog-agent-7.23.2-beta1-1-x86_64.msi"
	run(t, s)
}

type testNPMUpgradeFromBeta struct {
	testNPMInstallSuite
}

func (s *testNPMUpgradeFromBeta) TestNPMUpgradeFromBeta() {
	s.installPreviousAgentVersion(s.Env().RemoteHost)
	s.Require().True(s.isNPMInstalled(), "NPM should be installed")

	// upgrade to the new version
	s.upgradeAgent(s.Env().RemoteHost, s.AgentPackage)

	// run tests
	s.Require().True(s.isNPMInstalled(), "NPM should be installed")
	s.enableNPM()
	s.testNPMFunctional()
}

type testNPMInstallSuite struct {
	baseAgentMSISuite
	previousVersion string
	url             string
}

func (s *testNPMInstallSuite) TearDownSuite() {
	if tearDown, ok := any(&s.baseAgentMSISuite).(suite.TearDownAllSuite); ok {
		tearDown.TearDownSuite()
	}

	s.cleanupOnSuccessInDevMode()
}

func (s *testNPMInstallSuite) installPreviousAgentVersion(vm *components.RemoteHost, options ...windowsAgent.InstallAgentOption) *Tester {
	if s.url == "" {
		url, err := windowsAgent.GetStableMSIURL(s.previousVersion, "x86_64")
		s.Require().NoError(err, "should get MSI URL for version %s", s.previousVersion)
		s.url = url
	}
	previousAgentPackage := &windowsAgent.Package{
		Version: s.previousVersion,
		URL:     s.url,
	}
	return s.baseAgentMSISuite.installPreviousAgentVersion(vm, previousAgentPackage, options...)
}

func (s *testNPMInstallSuite) isNPMInstalled() bool {
	_, err := windowsCommon.GetServiceStatus(s.Env().RemoteHost, "ddnpm")
	return err == nil
}

func (s *testNPMInstallSuite) enableNPM() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)

	s.T().Log("Enabling NPM in config")
	configPath := filepath.Join(configRoot, "system-probe.yaml")
	config, err := s.readYamlConfig(host, configPath)
	s.Require().NoError(err)
	config["network_config"] = map[string]interface{}{
		"enabled": true,
	}
	err = s.writeYamlConfig(host, configPath, config)
	s.Require().NoError(err)

	// restart the agent
	s.T().Log("Restarting agent")
	err = windowsCommon.RestartService(host, "datadogagent")
	s.Require().NoError(err)
	s.T().Log("Agent restarted")
}

func (s *testNPMInstallSuite) testNPMFunctional() {
	host := s.Env().RemoteHost
	s.Run("npm running", func() {
		// services are running
		expectedServices := []string{"datadog-system-probe", "ddnpm"}
		for _, serviceName := range expectedServices {
			s.Assert().EventuallyWithT(func(c *assert.CollectT) {
				status, err := windowsCommon.GetServiceStatus(host, serviceName)
				require.NoError(c, err)
				assert.Equal(c, "Running", status, "%s should be running", serviceName)
			}, 1*time.Minute, 1*time.Second, "%s should be running", serviceName)
		}
	})
	s.Run("agent npm status", func() {
		client := s.NewTestClientForHost(host)
		status, err := client.GetJSONStatus()
		s.Require().NoError(err)
		s.Require().Contains(status, "systemProbeStats", "agent status should contain systemProbeStats")
		systemProbeStats := status["systemProbeStats"].(map[string]interface{})
		s.Require().NotContains(systemProbeStats, "Errors", "system probe status should not contain Errors")
	})
}

func (s *testNPMInstallSuite) upgradeAgent(host *components.RemoteHost, agentPackage *windowsAgent.Package, options ...windowsAgent.InstallAgentOption) {
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
	}
	installOpts = append(installOpts, options...)
	if !s.Run(fmt.Sprintf("upgrade to %s", agentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(host, installOpts...)
		s.Require().NoError(err, "should upgrade to agent %s", agentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	if !s.Run(fmt.Sprintf("test %s", agentPackage.AgentVersion()), func() {
		// check version
		client := s.NewTestClientForHost(host)
		if !s.Run("running expected agent version", func() {
			installedVersion, err := client.GetAgentVersion()
			s.Require().NoError(err, "should get agent version")
			windowsAgent.TestAgentVersion(s.T(), agentPackage.AgentVersion(), installedVersion)
		}) {
			s.T().FailNow()
		}
		// check no errors
		RequireAgentRunningWithNoErrors(s.T(), client)
	}) {
		s.T().FailNow()
	}
}
