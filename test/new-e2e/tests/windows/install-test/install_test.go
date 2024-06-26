// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	boundport "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestInstall(t *testing.T) {
	s := &testInstallSuite{}
	run(t, s)
}

type testInstallSuite struct {
	baseAgentMSISuite
}

func (s *testInstallSuite) TestInstall() {
	vm := s.Env().RemoteHost

	// initialize test helper
	t := s.newTester(vm)

	// install the agent
	remoteMSIPath := s.installAgentPackage(vm, s.AgentPackage)

	// run tests
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}
	s.testCodeSignatures(t, remoteMSIPath)

	s.uninstallAgentAndRunUninstallTests(t)
}

// testCodeSignatures checks the code signatures of the installed files.
// The same MSI is used in all tests so this test is only done once, in TestInstall.
func (s *testInstallSuite) testCodeSignatures(t *Tester, remoteMSIPath string) {
	// Get a list of files, and sort them into DD signed and other signed
	root, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	s.Require().NoError(err)
	paths := getExpectedSignedFilesForAgentMajorVersion(t.expectedAgentMajorVersion)
	for i, path := range paths {
		paths[i] = root + path
	}
	ddSigned := []string{}
	otherSigned := []string{}
	for _, path := range paths {
		if strings.Contains(path, "embedded3") {
			// As of 7.5?, the embedded Python3 should be signed by Python, not Datadog
			// We still build our own Python2, so we need to check that still
			otherSigned = append(otherSigned, path)
		} else {
			ddSigned = append(ddSigned, path)
		}
	}
	// MSI is signed by Datadog
	if remoteMSIPath != "" {
		ddSigned = append(ddSigned, remoteMSIPath)
	}
	windowsAgent.TestValidDatadogCodeSignatures(s.T(), t.host, ddSigned)

	// Check other signed files
	s.Run("check other signed files", func() {
		verify, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.VerifyCodeSignature, true)
		if !verify {
			s.T().Skip("skipping code signature verification")
		}
		for _, path := range otherSigned {
			subject := ""
			if strings.Contains(path, "embedded3") {
				subject = "CN=Python Software Foundation, O=Python Software Foundation, L=Beaverton, S=Oregon, C=US"
			} else {
				s.Assert().Failf("unexpected signed executable", "unexpected signed executable %s", path)
			}
			sig, err := windowsCommon.GetAuthenticodeSignature(t.host, path)
			if !s.Assert().NoError(err, "should get authenticode signature for %s", path) {
				continue
			}
			s.Assert().Truef(sig.Valid(), "signature should be valid for %s", path)
			s.Assert().Equalf(sig.SignerCertificate.Subject, subject, "subject should match for %s", path)
		}
	})
}

func TestInstallAltDir(t *testing.T) {
	s := &testInstallAltDirSuite{}
	run(t, s)
}

type testInstallAltDirSuite struct {
	baseAgentMSISuite
}

func (s *testInstallAltDirSuite) TestInstallAltDir() {
	vm := s.Env().RemoteHost

	installPath := `C:\altdir`
	configRoot := `C:\altconfroot`

	// initialize test helper
	t := s.newTester(vm,
		WithExpectedInstallPath(installPath),
		WithExpectedConfigRoot(configRoot),
	)

	// install the agent
	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithProjectLocation(installPath),
		windowsAgent.WithApplicationDataDirectory(configRoot),
	)

	// run tests
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

func TestRepair(t *testing.T) {
	s := &testRepairSuite{}
	run(t, s)
}

type testRepairSuite struct {
	baseAgentMSISuite
}

// TC-INS-001
func (s *testRepairSuite) TestRepair() {
	vm := s.Env().RemoteHost

	// initialize test helper
	t := s.newTester(vm)

	// install the agent
	_ = s.installAgentPackage(vm, s.AgentPackage)
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.AgentPackage.AgentVersion())

	err := windowsCommon.StopService(t.host, "DatadogAgent")
	s.Require().NoError(err)

	// Corrupt the install
	installPath, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	s.Require().NoError(err)
	err = t.host.Remove(filepath.Join(installPath, "bin", "agent.exe"))
	s.Require().NoError(err)
	err = t.host.RemoveAll(filepath.Join(installPath, "embedded3"))
	s.Require().NoError(err)
	// delete config files to ensure repair restores them
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(t.host)
	s.Require().NoError(err)
	err = t.host.RemoveAll(filepath.Join(configRoot, "runtime-security.d"))
	s.Require().NoError(err)

	// Run Repair through the MSI
	if !s.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(s.OutputDir, "repair.log"))
		s.Require().NoError(err)
	}) {
		s.T().FailNow()
	}

	// run tests, agent should function normally after repair
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

func TestInstallOpts(t *testing.T) {
	s := &testInstallOptsSuite{}
	run(t, s)
}

type testInstallOptsSuite struct {
	baseAgentMSISuite
}

// TC-INS-003
// tests that the installer options are set correctly in the agent config.
// This test toes the line between testing the installer and the agent. The installer
// already has unit-test coverage for the config replacement, so it is somewhat redundant to
// test it here.
// TODO: It would be better for the cmd_port binding test to be done by a regular Agent E2E test.
func (s *testInstallOptsSuite) TestInstallOpts() {
	vm := s.Env().RemoteHost

	cmdPort := 4999

	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithAPIKey("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		windowsAgent.WithPackage(s.AgentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "install.log")),
		windowsAgent.WithTags("k1:v1,k2:v2"),
		windowsAgent.WithHostname("win-installopts"),
		windowsAgent.WithCmdPort(fmt.Sprintf("%d", cmdPort)),
		windowsAgent.WithProxyHost("proxy.foo.com"),
		windowsAgent.WithProxyPort("1234"),
		windowsAgent.WithProxyUser("puser"),
		windowsAgent.WithProxyPassword("ppass"),
		windowsAgent.WithSite("eu"),
		windowsAgent.WithDdURL("https://someurl.datadoghq.com"),
		windowsAgent.WithLogsDdURL("https://logs.someurl.datadoghq.com"),
		windowsAgent.WithProcessDdURL("https://process.someurl.datadoghq.com"),
		windowsAgent.WithTraceDdURL("https://trace.someurl.datadoghq.com"),
	}

	_ = s.installAgentPackage(vm, s.AgentPackage, installOpts...)
	RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.AgentPackage.AgentVersion())

	// read the config file and check the options
	confYaml, err := s.readAgentConfig(vm)
	s.Require().NoError(err)

	assert.Contains(s.T(), confYaml, "hostname")
	assert.Equal(s.T(), "win-installopts", confYaml["hostname"], "hostname should match")
	assert.Contains(s.T(), confYaml, "api_key")
	assert.Equal(s.T(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", confYaml["api_key"], "api_key should match")
	assert.Contains(s.T(), confYaml, "tags")
	assert.ElementsMatch(s.T(), []string{"k1:v1", "k2:v2"}, confYaml["tags"], "tags should match")
	assert.Contains(s.T(), confYaml, "cmd_port")
	assert.Equal(s.T(), cmdPort, confYaml["cmd_port"], "cmd_port should match")
	assert.Contains(s.T(), confYaml, "site")
	assert.Equal(s.T(), "eu", confYaml["site"], "site should match")
	assert.Contains(s.T(), confYaml, "dd_url")
	assert.Equal(s.T(), "https://someurl.datadoghq.com", confYaml["dd_url"], "dd_url should match")

	if assert.Contains(s.T(), confYaml, "proxy") {
		// https proxy conf does use http:// in the URL
		// https://docs.datadoghq.com/agent/configuration/proxy/?tab=windows
		proxyConf := confYaml["proxy"].(map[string]interface{})
		assert.Contains(s.T(), proxyConf, "https")
		assert.Equal(s.T(), "http://puser:ppass@proxy.foo.com:1234/", proxyConf["https"], "https proxy should match")
		assert.Contains(s.T(), proxyConf, "http")
		assert.Equal(s.T(), "http://puser:ppass@proxy.foo.com:1234/", proxyConf["http"], "http proxy should match")
	}

	if assert.Contains(s.T(), confYaml, "logs_config") {
		logsConf := confYaml["logs_config"].(map[string]interface{})
		assert.Contains(s.T(), logsConf, "logs_dd_url")
		assert.Equal(s.T(), "https://logs.someurl.datadoghq.com", logsConf["logs_dd_url"], "logs_dd_url should match")
	}

	if assert.Contains(s.T(), confYaml, "process_config") {
		processConf := confYaml["process_config"].(map[string]interface{})
		assert.Contains(s.T(), processConf, "process_dd_url")
		assert.Equal(s.T(), "https://process.someurl.datadoghq.com", processConf["process_dd_url"], "process_dd_url should match")
	}

	if assert.Contains(s.T(), confYaml, "apm_config") {
		apmConf := confYaml["apm_config"].(map[string]interface{})
		assert.Contains(s.T(), apmConf, "apm_dd_url")
		assert.Equal(s.T(), "https://trace.someurl.datadoghq.com", apmConf["apm_dd_url"], "apm_dd_url should match")
	}

	// check that agent is listening on the new bound port
	var boundPort boundport.BoundPort
	s.Require().EventuallyWithTf(func(c *assert.CollectT) {
		pid, err := windowsCommon.GetServicePID(vm, "DatadogAgent")
		if !assert.NoError(c, err) {
			return
		}
		boundPort, err = common.GetBoundPort(vm, cmdPort)
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotNil(c, boundPort, "port %d should be bound", cmdPort) {
			return
		}
		assert.Equalf(c, pid, boundPort.PID(), "port %d should be bound by the agent", cmdPort)
	}, 1*time.Minute, 500*time.Millisecond, "port %d should be bound by the agent", cmdPort)
	s.Require().EqualValues("127.0.0.1", boundPort.LocalAddress(), "agent should only be listening locally")

	s.cleanupOnSuccessInDevMode()
}

func TestInstallFail(t *testing.T) {
	s := &testInstallFailSuite{}
	run(t, s)
}

type testInstallFailSuite struct {
	baseAgentMSISuite
}

// TC-INS-007
//
// Runs the installer with WIXFAILWHENDEFERRED=1 to trigger a failure at the very end of the installer.
func (s *testInstallFailSuite) TestInstallFail() {
	vm := s.Env().RemoteHost

	// run installer with failure flag
	if !s.Run(fmt.Sprintf("install %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "install.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// currently the install failure tests are the same as the uninstall tests
	t := s.newTester(vm)
	t.TestUninstallExpectations(s.T())
}
