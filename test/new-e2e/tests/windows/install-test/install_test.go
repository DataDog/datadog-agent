// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	boundport "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/bound-port"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type agentMSISuite struct {
	windows.BaseAgentInstallerSuite[environments.Host]
	beforeInstall *windowsCommon.FileSystemSnapshot
}

func TestMSI(t *testing.T) {
	opts := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(componentos.WindowsDefault)),
	))}

	agentPackage, err := windowsAgent.GetPackageFromEnv()
	if err != nil {
		t.Fatalf("failed to get MSI URL from env: %v", err)
	}
	t.Logf("Using Agent: %#v", agentPackage)

	majorVersion := strings.Split(agentPackage.Version, ".")[0]

	// Set stack name to avoid conflicts with other tests
	// Include channel if we're not running in a CI pipeline.
	// E2E auto includes the pipeline ID in the stack name, so we don't need to do that here.
	stackNameChannelPart := ""
	if agentPackage.PipelineID == "" && agentPackage.Channel != "" {
		stackNameChannelPart = fmt.Sprintf("-%s", agentPackage.Channel)
	}
	stackNameCIJobPart := ""
	ciJobID := os.Getenv("CI_JOB_ID")
	if ciJobID != "" {
		stackNameCIJobPart = fmt.Sprintf("-%s", os.Getenv("CI_JOB_ID"))
	}
	opts = append(opts, e2e.WithStackName(fmt.Sprintf("windows-msi-test-v%s-%s%s%s", majorVersion, agentPackage.Arch, stackNameChannelPart, stackNameCIJobPart)))

	s := &agentMSISuite{}

	// Include the agent major version in the test name so junit reports will differentiate the tests
	t.Run(fmt.Sprintf("Agent v%s", majorVersion), func(t *testing.T) {
		e2e.Run(t, s, opts...)
	})
}

func (is *agentMSISuite) prepareHost() {
	vm := is.Env().RemoteHost

	if !is.Run("prepare VM", func() {
		is.Run("disable defender", func() {
			err := windowsCommon.DisableDefender(vm)
			is.Require().NoError(err, "should disable defender")
		})
	}) {
		is.T().Fatal("failed to prepare VM")
	}
}

// TODO: this isn't called before tabular tests (e.g. TestAgentUser/...)
func (is *agentMSISuite) BeforeTest(suiteName, testName string) {
	if beforeTest, ok := any(&is.BaseAgentInstallerSuite).(suite.BeforeTest); ok {
		beforeTest.BeforeTest(suiteName, testName)
	}

	vm := is.Env().RemoteHost
	var err error
	// If necessary (for example for parallelization), store the snapshot per suite/test in a map
	is.beforeInstall, err = windowsCommon.NewFileSystemSnapshot(vm, SystemPaths())
	is.Require().NoError(err)

	// Clear the event logs before each test
	for _, logName := range []string{"System", "Application"} {
		is.T().Logf("Clearing %s event log", logName)
		err = windowsCommon.ClearEventLog(vm, logName)
		is.Require().NoError(err, "should clear %s event log", logName)
	}
}

// TODO: this isn't called after tabular tests (e.g. TestAgentUser/...)
func (is *agentMSISuite) AfterTest(suiteName, testName string) {
	if afterTest, ok := any(&is.BaseAgentInstallerSuite).(suite.AfterTest); ok {
		afterTest.AfterTest(suiteName, testName)
	}

	if is.T().Failed() {
		// If the test failed, export the event logs for debugging
		vm := is.Env().RemoteHost
		for _, logName := range []string{"System", "Application"} {
			// collect the full event log as an evtx file
			is.T().Logf("Exporting %s event log", logName)
			outputPath := filepath.Join(is.OutputDir, fmt.Sprintf("%s.evtx", logName))
			err := windowsCommon.ExportEventLog(vm, logName, outputPath)
			is.Assert().NoError(err, "should export %s event log", logName)
			// Log errors and warnings to the screen for easy access
			out, err := windowsCommon.GetEventLogErrorsAndWarnings(vm, logName)
			if is.Assert().NoError(err, "should get errors and warnings from %s event log", logName) && out != "" {
				is.T().Logf("Errors and warnings from %s event log:\n%s", logName, out)
			}
		}
	}
}

// cleanupAgent fully removes the agent from the VM, MSI, config, agent user, etc,
// to create a clean slate for the next test to run on the same host/stack.
func (is *agentMSISuite) cleanupAgent() {
	host := is.Env().RemoteHost
	t := is.T()

	// The uninstaller removes the registry keys that point to the install parameters,
	// so we collect them prior to uninstalling and then use defer to perform the actions.

	// remove the agent user
	_, agentUserName, err := windowsAgent.GetAgentUserFromRegistry(host)
	if err == nil {
		defer func() {
			t.Logf("Removing agent user: %s", agentUserName)
			err = windowsCommon.RemoveLocalUser(host, agentUserName)
			require.NoError(t, err)
		}()
	}

	// remove the agent config
	configDir, err := windowsAgent.GetConfigRootFromRegistry(host)
	if err == nil {
		defer func() {
			t.Logf("Removing agent configuration files: %s", configDir)
			err = host.RemoveAll(configDir)
			require.NoError(t, err)
		}()
	}

	// uninstall the agent
	_, err = windowsAgent.GetDatadogAgentProductCode(host)
	if err == nil {
		defer func() {
			t.Logf("Uninstalling Datadog Agent")
			err = windowsAgent.UninstallAgent(host, filepath.Join(is.OutputDir, "uninstall.log"))
			require.NoError(t, err)
		}()
	}
}

// cleanupOnSuccessInDevMode runs clean tasks on the host when running in DevMode. This makes it
// easier to run subsequent tests on the same host without having to manually clean up.
// This is not necessary outside of DevMode because the VM is destroyed after each test run.
// Cleanup is not run if the test failed, to allow for manual inspection of the VM.
func (is *agentMSISuite) cleanupOnSuccessInDevMode() {
	if !is.IsDevMode() || is.T().Failed() {
		return
	}
	is.T().Log("Running DevMode cleanup tasks")
	is.cleanupAgent()
}

func (is *agentMSISuite) TestInstall() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// initialize test helper
	t := is.newTester(vm)

	// install the agent
	remoteMSIPath := is.installAgentPackage(vm, is.AgentPackage)

	// run tests
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	// Test the code signatures of the installed files.
	// The same MSI is used in all tests so only check it once here.
	root, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	is.Require().NoError(err)
	paths := getExpectedSignedFilesForAgentMajorVersion(t.expectedAgentMajorVersion)
	for i, path := range paths {
		paths[i] = root + path
	}
	if remoteMSIPath != "" {
		paths = append(paths, remoteMSIPath)
	}
	windowsAgent.TestValidDatadogCodeSignatures(is.T(), t.host, paths)

	is.uninstallAgentAndRunUninstallTests(t)
}

func (is *agentMSISuite) TestUpgrade() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// install previous version
	_ = is.installLastStable(vm)

	// upgrade to the new version
	if !is.Run(fmt.Sprintf("upgrade to %s", is.AgentPackage.AgentVersion()), func() {
		_, err := is.InstallAgent(vm,
			windowsAgent.WithPackage(is.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "upgrade.log")),
		)
		is.Require().NoError(err, "should upgrade to agent %s", is.AgentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	// run tests
	t := is.newTester(vm)
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(t)
}

// TC-INS-002
func (is *agentMSISuite) TestUpgradeRollback() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// install previous version
	previousTester := is.installLastStable(vm)

	// upgrade to the new version, but intentionally fail
	if !is.Run(fmt.Sprintf("upgrade to %s with rollback", is.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(is.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "upgrade.log")),
		)
		is.Require().Error(err, "should fail to install agent %s", is.AgentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	is.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	if !previousTester.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(previousTester)
}

// TC-INS-001
func (is *agentMSISuite) TestRepair() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	// initialize test helper
	t := is.newTester(vm)

	// install the agent
	_ = is.installAgentPackage(vm, is.AgentPackage)

	err := windowsCommon.StopService(t.host, "DatadogAgent")
	is.Require().NoError(err)

	// Corrupt the install
	installPath, err := windowsAgent.GetInstallPathFromRegistry(t.host)
	is.Require().NoError(err)
	err = t.host.Remove(filepath.Join(installPath, "bin", "agent.exe"))
	is.Require().NoError(err)
	err = t.host.RemoveAll(filepath.Join(installPath, "embedded3"))
	is.Require().NoError(err)

	// Run Repair through the MSI
	if !is.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(is.OutputDir, "repair.log"))
		is.Require().NoError(err)
	}) {
		is.T().FailNow()
	}

	// run tests, agent should function normally after repair
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	is.uninstallAgentAndRunUninstallTests(t)
}

// TC-INS-006
func (is *agentMSISuite) TestAgentUser() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	hostinfo, err := windowsCommon.GetHostInfo(vm)
	is.Require().NoError(err)

	domainPart := windowsCommon.NameToNetBIOSName(hostinfo.Hostname)

	tcs := []struct {
		testname       string
		builtinaccount bool
		username       string
		expectedDomain string
		expectedUser   string
	}{
		{"user_only", false, "testuser", domainPart, "testuser"},
		{"dotslash_user", false, ".\\testuser", domainPart, "testuser"},
		{"domain_user", false, fmt.Sprintf("%s\\testuser", domainPart), domainPart, "testuser"},
		{"LocalSystem", true, "LocalSystem", "NT AUTHORITY", "SYSTEM"},
		{"SYSTEM", true, "SYSTEM", "NT AUTHORITY", "SYSTEM"},
	}
	for _, tc := range tcs {
		if !is.Run(tc.testname, func() {
			// subtest needs a new output dir
			is.OutputDir, err = runner.GetTestOutputDir(runner.GetProfile(), is.T())
			is.Require().NoError(err, "should get output dir")

			// initialize test helper
			t := is.newTester(vm,
				WithExpectedAgentUser(tc.expectedDomain, tc.expectedUser),
			)

			// install the agent
			_ = is.installAgentPackage(vm, is.AgentPackage,
				windowsAgent.WithAgentUser(tc.username),
			)

			// run tests
			if !t.TestInstallExpectations(is.T()) {
				is.T().FailNow()
			}

			is.uninstallAgentAndRunUninstallTests(t)
		}) {
			is.T().FailNow()
		}
	}
}

// TC-INS-003
// tests that the installer options are set correctly in the agent config.
// This test toes the line between testing the installer and the agent. The installer
// already has unit-test coverage for the config replacement, so it is somewhat redundant to
// test it here.
// TODO: It would be better for the cmd_port binding test to be done by a regular Agent E2E test.
func (is *agentMSISuite) TestInstallOpts() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	cmdPort := 4999

	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithAPIKey("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		windowsAgent.WithPackage(is.AgentPackage),
		windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "install.log")),
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

	_ = is.installAgentPackage(vm, is.AgentPackage, installOpts...)

	// read the config file and check the options
	confYaml, err := is.readAgentConfig(vm)
	is.Require().NoError(err)

	assert.Contains(is.T(), confYaml, "hostname")
	assert.Equal(is.T(), "win-installopts", confYaml["hostname"], "hostname should match")
	assert.Contains(is.T(), confYaml, "api_key")
	assert.Equal(is.T(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", confYaml["api_key"], "api_key should match")
	assert.Contains(is.T(), confYaml, "tags")
	assert.ElementsMatch(is.T(), []string{"k1:v1", "k2:v2"}, confYaml["tags"], "tags should match")
	assert.Contains(is.T(), confYaml, "cmd_port")
	assert.Equal(is.T(), cmdPort, confYaml["cmd_port"], "cmd_port should match")
	assert.Contains(is.T(), confYaml, "site")
	assert.Equal(is.T(), "eu", confYaml["site"], "site should match")
	assert.Contains(is.T(), confYaml, "dd_url")
	assert.Equal(is.T(), "https://someurl.datadoghq.com", confYaml["dd_url"], "dd_url should match")

	if assert.Contains(is.T(), confYaml, "proxy") {
		// https proxy conf does use http:// in the URL
		// https://docs.datadoghq.com/agent/configuration/proxy/?tab=windows
		proxyConf := confYaml["proxy"].(map[string]interface{})
		assert.Contains(is.T(), proxyConf, "https")
		assert.Equal(is.T(), "http://puser:ppass@proxy.foo.com:1234/", proxyConf["https"], "https proxy should match")
		assert.Contains(is.T(), proxyConf, "http")
		assert.Equal(is.T(), "http://puser:ppass@proxy.foo.com:1234/", proxyConf["http"], "http proxy should match")
	}

	if assert.Contains(is.T(), confYaml, "logs_config") {
		logsConf := confYaml["logs_config"].(map[string]interface{})
		assert.Contains(is.T(), logsConf, "logs_dd_url")
		assert.Equal(is.T(), "https://logs.someurl.datadoghq.com", logsConf["logs_dd_url"], "logs_dd_url should match")
	}

	if assert.Contains(is.T(), confYaml, "process_config") {
		processConf := confYaml["process_config"].(map[string]interface{})
		assert.Contains(is.T(), processConf, "process_dd_url")
		assert.Equal(is.T(), "https://process.someurl.datadoghq.com", processConf["process_dd_url"], "process_dd_url should match")
	}

	if assert.Contains(is.T(), confYaml, "apm_config") {
		apmConf := confYaml["apm_config"].(map[string]interface{})
		assert.Contains(is.T(), apmConf, "apm_dd_url")
		assert.Equal(is.T(), "https://trace.someurl.datadoghq.com", apmConf["apm_dd_url"], "apm_dd_url should match")
	}

	// check that agent is listening on the new bound port
	var boundPort boundport.BoundPort
	is.Require().EventuallyWithTf(func(c *assert.CollectT) {
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
	is.Require().EqualValues("127.0.0.1", boundPort.LocalAddress(), "agent should only be listening locally")

	is.cleanupOnSuccessInDevMode()
}

// TestSubServicesOpts tests that the agent installer can configure the subservices.
// TODO: Once E2E's Agent interface supports providing MSI installer options these tests
// should be moved to regular Agent E2E tests for each subservice.
func (is *agentMSISuite) TestSubServicesOpts() {
	vm := is.Env().RemoteHost
	is.prepareHost()

	tcs := []struct {
		testname string
		// it's surprising but we do not have an installer option for enabling NPM/system-probe.
		logsEnabled    bool
		processEnabled bool
		apmEnabled     bool
	}{
		// TC-INS-004
		{"all-subservices", true, true, true},
		// TC-INS-005
		{"no-subservices", false, false, false},
	}
	for _, tc := range tcs {
		if !is.Run(tc.testname, func() {

			installOpts := []windowsAgent.InstallAgentOption{
				windowsAgent.WithLogsEnabled(strconv.FormatBool(tc.logsEnabled)),
				// set both process agent options so we can check if process-agent is running or not
				windowsAgent.WithProcessEnabled(strconv.FormatBool(tc.processEnabled)),
				windowsAgent.WithProcessDiscoveryEnabled(strconv.FormatBool(tc.processEnabled)),
				windowsAgent.WithAPMEnabled(strconv.FormatBool(tc.apmEnabled)),
			}
			_ = is.installAgentPackage(vm, is.AgentPackage, installOpts...)

			// read the config file and check the options
			confYaml, err := is.readAgentConfig(vm)
			is.Require().NoError(err)

			assert.Contains(is.T(), confYaml, "logs_enabled", "logs_enabled should be present in the config")
			assert.Equal(is.T(), tc.logsEnabled, confYaml["logs_enabled"], "logs_enabled should match")

			if assert.Contains(is.T(), confYaml, "process_config", "process_config should be present in the config") {
				processConf := confYaml["process_config"].(map[string]interface{})
				if assert.Contains(is.T(), processConf, "process_collection", "process_collection should be present in process_config") {
					processCollectionConf := processConf["process_collection"].(map[string]interface{})
					assert.Contains(is.T(), processCollectionConf, "enabled", "enabled should be present in process_collection")
					assert.Equal(is.T(), tc.processEnabled, processCollectionConf["enabled"], "process_collection enabled should match")
				}
				if assert.Contains(is.T(), processConf, "process_discovery", "process_discovery should be present in process_config") {
					processDiscoveryConf := processConf["process_discovery"].(map[string]interface{})
					assert.Contains(is.T(), processDiscoveryConf, "enabled", "enabled should be present in process_discovery")
					assert.Equal(is.T(), tc.processEnabled, processDiscoveryConf["enabled"], "process_discovery enabled should match")
				}
			}

			if assert.Contains(is.T(), confYaml, "apm_config", "apm_config should be present in the config") {
				apmConf := confYaml["apm_config"].(map[string]interface{})
				assert.Contains(is.T(), apmConf, "enabled", "enabled should be present in apm_config")
				assert.Equal(is.T(), tc.apmEnabled, apmConf["enabled"], "apm_config enabled should match")
			}

			tcs := []struct {
				serviceName string
				enabled     bool
			}{
				// NOTE: Even with processEnabled=false the Agent will start process-agent because container_collection is
				//       enabled by default. We do not have an installer option to control this process-agent setting.
				//       However, process-agent will exit soon after starting because there's no container environment installed
				//       and the other options are disabled.
				{"datadog-process-agent", tc.processEnabled},
				{"datadog-trace-agent", tc.apmEnabled},
			}
			for _, tc := range tcs {
				assert.EventuallyWithT(is.T(), func(c *assert.CollectT) {
					status, err := windowsCommon.GetServiceStatus(vm, tc.serviceName)
					require.NoError(c, err)
					if tc.enabled {
						assert.Equal(c, "Running", status, "%s should be running", tc.serviceName)
					} else {
						assert.Equal(c, "Stopped", status, "%s should be stopped", tc.serviceName)
					}
				}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", tc.serviceName)
			}
		}) {
			is.T().FailNow()
		}
		// clean the host between test runs
		is.cleanupOnSuccessInDevMode()
	}
}

func (is *agentMSISuite) readYamlConfig(host *components.RemoteHost, path string) (map[string]any, error) {
	config, err := host.ReadFile(path)
	if err != nil {
		return nil, err
	}
	confYaml := make(map[string]any)
	err = yaml.Unmarshal(config, &confYaml)
	if err != nil {
		return nil, err
	}
	return confYaml, nil
}

func (is *agentMSISuite) readAgentConfig(host *components.RemoteHost) (map[string]any, error) {
	confDir, err := windowsAgent.GetConfigRootFromRegistry(host)
	if err != nil {
		return nil, err
	}
	configFilePath := filepath.Join(confDir, "datadog.yaml")
	return is.readYamlConfig(host, configFilePath)
}

func (is *agentMSISuite) uninstallAgentAndRunUninstallTests(t *Tester) bool {
	return is.T().Run("uninstall the agent", func(tt *testing.T) {
		// Get config dir from registry before uninstalling
		configDir, err := windowsAgent.GetConfigRootFromRegistry(t.host)
		require.NoError(tt, err)
		tt.Cleanup(func() {
			// remove the agent config for a cleaner uninstall
			tt.Logf("Removing agent configuration files")
			err = t.host.RemoveAll(configDir)
			require.NoError(tt, err)
		})

		if !tt.Run("uninstall", func(tt *testing.T) {
			err := windowsAgent.UninstallAgent(t.host, filepath.Join(is.OutputDir, "uninstall.log"))
			require.NoError(tt, err, "should uninstall the agent")
		}) {
			tt.Fatal("uninstall failed")
		}

		AssertDoesNotRemoveSystemFiles(is.T(), is.Env().RemoteHost, is.beforeInstall)

		t.TestUninstallExpectations(tt)
	})
}

func (is *agentMSISuite) newTester(vm *components.RemoteHost, options ...TesterOption) *Tester {
	testerOpts := []TesterOption{
		WithAgentPackage(is.AgentPackage),
	}
	testerOpts = append(testerOpts, options...)
	t, err := NewTester(is.T(), vm, testerOpts...)
	is.Require().NoError(err, "should create tester")
	return t
}

func (is *agentMSISuite) installAgentPackage(vm *components.RemoteHost, agentPackage *windowsAgent.Package, installOptions ...windowsAgent.InstallAgentOption) string {
	remoteMSIPath := ""
	var err error

	// install the agent
	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(agentPackage),
		// default log file, can be overridden
		windowsAgent.WithInstallLogFile(filepath.Join(is.OutputDir, "install.log")),
		// trace-agent requires a valid API key
		windowsAgent.WithValidAPIKey(),
	}
	installOpts = append(installOpts, installOptions...)
	if !is.Run(fmt.Sprintf("install %s", agentPackage.AgentVersion()), func() {
		remoteMSIPath, err = is.InstallAgent(vm, installOpts...)
		is.Require().NoError(err, "should install agent %s", agentPackage.AgentVersion())
	}) {
		is.T().FailNow()
	}

	return remoteMSIPath
}

// installLastStable installs the last stable agent package on the VM, runs tests, and returns the Tester
func (is *agentMSISuite) installLastStable(vm *components.RemoteHost, options ...windowsAgent.InstallAgentOption) *Tester {

	previousAgentPackage, err := windowsAgent.GetLastStablePackageFromEnv()
	is.Require().NoError(err, "should get last stable agent package from env")

	// create the tester
	t := is.newTester(vm,
		WithAgentPackage(previousAgentPackage),
		WithPreviousVersion(),
	)

	_ = is.installAgentPackage(vm, previousAgentPackage, options...)

	// Ensure the agent is functioning properly to provide a proper foundation for the test
	if !t.TestInstallExpectations(is.T()) {
		is.T().FailNow()
	}

	return t
}
