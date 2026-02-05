// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	boundport "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestInstall(t *testing.T) {
	s := &testInstallSuite{}
	Run(t, s)
}

type testInstallSuite struct {
	baseAgentMSISuite
}

func (s *testInstallSuite) TestInstall() {
	vm := s.Env().RemoteHost

	// initialize test helper
	t := s.newTester(vm)

	err := vm.MkdirAll(windowsAgent.DefaultConfigRoot)
	s.Require().NoErrorf(err, "could not create default config root")

	// create dummy files with known value to be replaced
	filesThatNeedToBeReplaced := []struct {
		path    string
		content string
	}{
		{
			path:    filepath.Join(windowsAgent.DefaultConfigRoot, "auth_token"),
			content: "F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0",
		},
		{
			path:    filepath.Join(windowsAgent.DefaultConfigRoot, "ipc_cert.pem"),
			content: "0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F",
		},
	}

	for _, f := range filesThatNeedToBeReplaced {
		_, err = vm.WriteFile(f.path, []byte(f.content))
		s.Require().NoErrorf(err, "could not write %s", f.path)
	}

	// install the agent
	remoteMSIPath := s.installAgentPackage(vm, s.AgentPackage)

	// check system dir permissions are same after install
	s.Run("install does not change system file permissions", func() {
		AssertDoesNotChangePathPermissions(s.T(), vm, s.beforeInstallPerms)
	})

	// run tests
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}
	s.testCodeSignatures(t, remoteMSIPath)
	for _, f := range filesThatNeedToBeReplaced {
		newFileContent, err := vm.ReadFile(f.path)
		s.Require().NoErrorf(err, "could not read %s", f.path)
		s.Assert().NotEqual(strings.TrimSpace(string(newFileContent)), strings.TrimSpace(f.content))
	}
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
	ddSigned = append(ddSigned, paths...)
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
		s.Assert().Empty(otherSigned, "no other signed files to check")
		for _, path := range otherSigned {
			subject := ""
			// As of 7.63, the embedded Python3 is back to being signed by Datadog, so it's checked above
			// If we need to check it or other files again, we can add it back here
			// 	subject = "CN=Python Software Foundation, O=Python Software Foundation, L=Beaverton, S=Oregon, C=US"
			// } else {
			// 	s.Assert().Failf("unexpected signed executable", "unexpected signed executable %s", path)
			// }
			sig, err := windowsCommon.GetAuthenticodeSignature(t.host, path)
			if !s.Assert().NoError(err, "should get authenticode signature for %s", path) {
				continue
			}
			s.Assert().Truef(sig.Valid(), "signature should be valid for %s", path)
			s.Assert().Equalf(sig.SignerCertificate.Subject, subject, "subject should match for %s", path)
		}
	})
}

// TestInstallExistingAltDir installs the agent to an existing directory and
// checks that the files are not removed
func TestInstallExistingAltDir(t *testing.T) {
	s := &testInstallExistingAltDirSuite{}
	Run(t, s)
}

type testInstallExistingAltDirSuite struct {
	baseAgentMSISuite
}

func (s *testInstallExistingAltDirSuite) TestInstallExistingAltDir() {
	vm := s.Env().RemoteHost

	installPath := `C:\altdir`
	configRoot := `C:\altconfroot`

	// create the install dir and add some files to it
	err := vm.MkdirAll(installPath)
	s.Require().NoError(err)
	fileData := map[string]string{
		"file1.txt":         "file1 data",
		"subdiir/file2.txt": "file2 data",
	}
	for file, data := range fileData {
		parent := filepath.Dir(file)
		if parent != "" {
			err := vm.MkdirAll(filepath.Join(installPath, filepath.Dir(file)))
			s.Require().NoError(err)
		}
		_, err = vm.WriteFile(filepath.Join(installPath, file), []byte(data))
		s.Require().NoError(err)
	}

	// install the agent
	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithProjectLocation(installPath),
		windowsAgent.WithApplicationDataDirectory(configRoot),
	)

	// uninstall the agent
	s.Require().True(
		s.uninstallAgent(),
	)

	// ensure the install dir and files are still there
	for file, data := range fileData {
		contents, err := vm.ReadFile(filepath.Join(installPath, file))
		if s.Assert().NoError(err, "file %s should still exist", file) {
			assert.Equal(s.T(), string(data), string(contents), "file %s should still have the same contents", file)
		}
	}
	// ensure the agent dirs are gone
	removedPaths := []string{
		filepath.Join(installPath, "bin"),
		filepath.Join(installPath, "embedded2"),
		filepath.Join(installPath, "embedded3"),
	}
	for _, path := range removedPaths {
		_, err := vm.Lstat(path)
		s.Require().Error(err, "path %s should be removed", path)
	}
}

func TestInstallAltDir(t *testing.T) {
	s := &testInstallAltDirSuite{}
	Run(t, s)
}

type testInstallAltDirSuite struct {
	baseAgentMSISuite
}

func (s *testInstallAltDirSuite) BeforeTest(suiteName, testName string) {
	// Remove users write permission from drive root, so install dir does not inherit writable permissions
	// Must be run before BaseSuite.BeforeTest takes the permission snapshot
	_, err := s.Env().RemoteHost.Execute(`icacls.exe C:/ /remove Users ; icacls.exe C:/ /grant Users:"(OI)(CI)(RX)"`)
	s.Require().NoError(err)

	s.baseAgentMSISuite.BeforeTest(suiteName, testName)
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

func TestInstallAltDirAndCorruptForUninstall(t *testing.T) {
	s := &testInstallAltDirAndCorruptForUninstallSuite{}
	Run(t, s)
}

type testInstallAltDirAndCorruptForUninstallSuite struct {
	baseAgentMSISuite
}

func (s *testInstallAltDirAndCorruptForUninstallSuite) TestInstallAltDirAndCorruptForUninstall() {
	vm := s.Env().RemoteHost

	installPath := `C:\altdir`
	configRoot := `C:\altconfroot`

	// install the agent
	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithProjectLocation(installPath),
		windowsAgent.WithApplicationDataDirectory(configRoot),
	)

	// remove registry key that contains install info to ensure uninstall succeeds
	// with a corrupted install
	err := windowsCommon.DeleteRegistryKey(vm, windowsAgent.RegistryKeyPath)
	s.Require().NoError(err)

	// uninstall the agent
	s.Require().True(
		s.uninstallAgent(),
	)

	_, err = vm.Lstat(installPath)
	s.Require().Error(err, "agent install dir should be removed")
	_, err = vm.Lstat(configRoot)
	s.Require().NoError(err, "agent config root dir should still exist")
}

func TestRepair(t *testing.T) {
	s := &testRepairSuite{}
	Run(t, s)
}

type testRepairSuite struct {
	baseAgentMSISuite
}

// TC-INS-001
func (s *testRepairSuite) TestRepair() {
	vm := s.Env().RemoteHost

	// initialize test helper
	t := s.newTester(vm)

	// install the agent - skip procdump since this test deletes agent files
	_ = s.installAgentPackageWithOptions(vm, s.AgentPackage, []PackageInstallOption{WithSkipProcdump()})
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
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(s.SessionOutputDir(), "repair.log"))
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
	Run(t, s)
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
		windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		windowsAgent.WithTags("k1:v1,k2:v2"),
		windowsAgent.WithHostname("win-installopts"),
		windowsAgent.WithCmdPort(strconv.Itoa(cmdPort)),
		windowsAgent.WithProxyHost("proxy.foo.com"),
		windowsAgent.WithProxyPort("1234"),
		windowsAgent.WithProxyUser("puser"),
		windowsAgent.WithProxyPassword("ppass"),
		windowsAgent.WithSite("eu"),
		windowsAgent.WithDdURL("https://someurl.datadoghq.com"),
		windowsAgent.WithLogsDdURL("https://logs.someurl.datadoghq.com"),
		windowsAgent.WithProcessDdURL("https://process.someurl.datadoghq.com"),
		windowsAgent.WithTraceDdURL("https://trace.someurl.datadoghq.com"),
		windowsAgent.WithRemoteUpdates("true"),
		windowsAgent.WithInfrastructureMode("basic"),
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

	assert.Contains(s.T(), confYaml, "remote_updates")
	assert.Equal(s.T(), true, confYaml["remote_updates"], "remote_updates should match")

	assert.Contains(s.T(), confYaml, "infrastructure_mode")
	assert.Equal(s.T(), "basic", confYaml["infrastructure_mode"], "infrastructure_mode should match")

	// check that agent is listening on the new bound port
	var boundPort boundport.BoundPort
	s.Require().EventuallyWithTf(func(c *assert.CollectT) {
		pid, err := windowsCommon.GetServicePID(vm, "DatadogAgent")
		if !assert.NoError(c, err) {
			return
		}
		boundPort, err = common.GetBoundPort(vm, "tcp", cmdPort)
		if !assert.NoError(c, err) {
			return
		}
		if !assert.NotNil(c, boundPort, "port tcp/%d should be bound", cmdPort) {
			return
		}
		assert.Equalf(c, pid, boundPort.PID(), "port tcp/%d should be bound by the agent", cmdPort)
	}, 1*time.Minute, 500*time.Millisecond, "port tcp/%d should be bound by the agent", cmdPort)
	s.Require().EqualValues("127.0.0.1", boundPort.LocalAddress(), "agent should only be listening locally")

	s.cleanupOnSuccessInDevMode()
}

func TestInstallFail(t *testing.T) {
	s := &testInstallFailSuite{}
	Run(t, s)
}

type testInstallFailSuite struct {
	baseAgentMSISuite
}

func (s *testInstallFailSuite) BeforeTest(suiteName, testName string) {
	if beforeTest, ok := any(&s.baseAgentMSISuite).(suite.BeforeTest); ok {
		beforeTest.BeforeTest(suiteName, testName)
	}

	host := s.Env().RemoteHost

	// Create another dir in the parent dir of install path to simulate having another Datadog
	// product installed, such as the APM Profiler, as the path already existing impacts how
	// Windows Installer treats the path (see SystemPathsForPermissionsValidation())
	basePath := `C:\Program Files\Datadog`
	err := host.MkdirAll(filepath.Join(basePath, "NotARealProduct"))
	s.Require().NoError(err)
	// collect perms and add to map for later comparison
	beforeInstall, err := SnapshotPermissionsForPaths(host, []string{basePath})
	s.Require().NoError(err)
	s.beforeInstallPerms[basePath] = beforeInstall[basePath]
}

// TC-INS-007
//
// Runs the installer with WIXFAILWHENDEFERRED=1 to trigger a failure at the very end of the installer.
func (s *testInstallFailSuite) TestInstallFail() {
	vm := s.Env().RemoteHost

	// run installer with failure flag
	if !s.Run("install "+s.AgentPackage.AgentVersion(), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithValidAPIKey(),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "install.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// currently the install failure tests are the same as the uninstall tests
	t := s.newTester(vm)
	t.TestUninstallExpectations(s.T())

	// check system dir permissions are same after rollback
	s.Run("rollback does not change system file permissions", func() {
		AssertDoesNotChangePathPermissions(s.T(), vm, s.beforeInstallPerms)
	})
}

// TestInstallWithLanmanServerDisabled tests that the Agent can be installed when the LanmanServer service is disabled.
// This is the case in Windows Containers, but is not likely to be encountered in other environments.
func TestInstallWithLanmanServerDisabled(t *testing.T) {
	s := &testInstallWithLanmanServerDisabledSuite{}
	Run(t, s)
}

type testInstallWithLanmanServerDisabledSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithLanmanServerDisabledSuite) TestInstallWithLanmanServerDisabled() {
	vm := s.Env().RemoteHost

	// Disable LanmanServer service to prevent it from starting again, and then stop it
	_, err := vm.Execute("sc.exe config lanmanserver start= disabled")
	s.Require().NoError(err)
	err = windowsCommon.StopService(vm, "lanmanserver")
	s.Require().NoError(err)

	_ = s.installAgentPackage(vm, s.AgentPackage)
	RequireAgentRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm))
}

func TestInstallWithInstallOnlyFlag(t *testing.T) {
	s := &testInstallWithInstallOnlyFlagSuite{}
	Run(t, s)
}

type testInstallWithInstallOnlyFlagSuite struct {
	baseAgentMSISuite
}

func (s *testInstallWithInstallOnlyFlagSuite) TestInstallWithInstallOnlyFlag() {
	vm := s.Env().RemoteHost

	// install the agent with DD_INSTALL_ONLY=1
	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithInstallOnly("true"),
	)

	// Verify the agent was installed
	productCode, err := windowsAgent.GetDatadogAgentProductCode(vm)
	s.Require().NoError(err, "should find installed agent")
	s.Require().NotEmpty(productCode, "product code should not be empty")

	// Check that services are NOT running
	// Use the standard list of expected installed services
	for _, service := range servicetest.ExpectedInstalledServices() {
		status, err := windowsCommon.GetServiceStatus(vm, service)
		s.Require().NoError(err, "service %s should exist", service)
		s.Assert().NotEqual("Running", status,
			"service %s should not be running when DD_INSTALL_ONLY=1", service)
	}

	// Verify services can be started manually
	s.Run("services can be started manually", func() {
		err := windowsCommon.StartService(vm, "datadogagent")
		s.Require().NoError(err, "should be able to start datadogagent service")

		// Wait for service to start
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(vm, "datadogagent")
			assert.NoError(c, err, "should get service status")
			assert.Equal(c, "Running", status, "datadogagent should be running after manual start")
		}, 30*time.Second, 1*time.Second, "datadogagent service should start within 30 seconds")
	})

	// Clean up
	t := s.newTester(vm)
	s.uninstallAgentAndRunUninstallTests(t)
}
