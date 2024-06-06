// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	agentClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	agentClientParams "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	commonHelper "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	servicetest "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tester is a test helper for testing agent installations
type Tester struct {
	hostInfo          *windows.HostInfo
	host              *components.RemoteHost
	InstallTestClient *common.TestClient

	agentPackage      *windowsAgent.Package
	isPreviousVersion bool

	expectedUserName   string
	expectedUserDomain string

	expectedAgentVersion      string
	expectedAgentMajorVersion string

	expectedInstallPath string
	expectedConfigRoot  string
}

// TesterOption is a function that can be used to configure a Tester
type TesterOption func(*Tester)

// NewTester creates a new Tester
func NewTester(context e2e.Context, host *components.RemoteHost, opts ...TesterOption) (*Tester, error) {
	t := &Tester{}
	tt := context.T()

	var err error

	t.host = host
	t.hostInfo, err = windows.GetHostInfo(t.host)
	if err != nil {
		return nil, err
	}
	t.expectedUserName = windowsAgent.DefaultAgentUserName
	t.expectedUserDomain = windows.NameToNetBIOSName(t.hostInfo.Hostname)
	t.expectedInstallPath = windowsAgent.DefaultInstallPath
	t.expectedConfigRoot = windowsAgent.DefaultConfigRoot

	for _, opt := range opts {
		opt(t)
	}

	if t.expectedAgentVersion == "" {
		return nil, fmt.Errorf("expectedAgentVersion is required")
	}

	// Ensure the expected version is well formed
	if !tt.Run("validate input params", func(tt *testing.T) {
		if !windowsAgent.TestAgentVersion(tt, t.expectedAgentVersion, t.expectedAgentVersion) {
			tt.FailNow()
		}
	}) {
		tt.FailNow()
	}

	t.InstallTestClient = common.NewWindowsTestClient(context, t.host)
	t.InstallTestClient.Helper = commonHelper.NewWindowsHelperWithCustomPaths(t.expectedInstallPath, t.expectedConfigRoot)
	t.InstallTestClient.AgentClient, err = agentClient.NewHostAgentClientWithParams(
		context,
		t.host.HostOutput,
		agentClientParams.WithSkipWaitForAgentReady(),
		agentClientParams.WithAgentInstallPath(t.expectedInstallPath),
	)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// WithAgentPackage sets the agent package to be installed
func WithAgentPackage(agentPackage *windowsAgent.Package) TesterOption {
	return func(t *Tester) {
		t.agentPackage = agentPackage
		t.expectedAgentVersion = agentPackage.AgentVersion()
		t.expectedAgentMajorVersion = strings.Split(t.expectedAgentVersion, ".")[0]
	}
}

// WithPreviousVersion sets the Tester to expect a previous version of the agent to be installed
// and will not run all tests since expectations may have changed.
func WithPreviousVersion() TesterOption {
	return func(t *Tester) {
		t.isPreviousVersion = true
	}
}

// WithExpectedAgentUser sets the expected user the agent should run as
func WithExpectedAgentUser(domain string, user string) TesterOption {
	return func(t *Tester) {
		t.expectedUserDomain = domain
		t.expectedUserName = user
	}
}

// WithExpectedInstallPath sets the expected install path for the agent
func WithExpectedInstallPath(path string) TesterOption {
	return func(t *Tester) {
		t.expectedInstallPath = path
	}
}

// WithExpectedConfigRoot sets the expected config root for the agent
func WithExpectedConfigRoot(path string) TesterOption {
	return func(t *Tester) {
		t.expectedConfigRoot = path
	}
}

// ExpectPython2Installed returns true if the agent is expected to install Python2
func (t *Tester) ExpectPython2Installed() bool {
	return t.expectedAgentMajorVersion == "6"
}

// ExpectAPM returns true if the agent is expected to install APM
func (t *Tester) ExpectAPM() bool {
	return true
}

// ExpectCWS returns true if the agent is expected to install CWS
func (t *Tester) ExpectCWS() bool {
	// TODO: CWS on Windows isn't available yet
	return false
}

// runTestsForKitchenCompat runs several tests that were copied over from the kitchen tests.
// Many if not all of these should be independent E2E tests and not part of the installer
// tests, but they have not been converted yet.
func (t *Tester) runTestsForKitchenCompat(tt *testing.T) {
	tt.Run("agent runtime behavior", func(tt *testing.T) {
		common.CheckAgentStops(tt, t.InstallTestClient)
		common.CheckAgentRestarts(tt, t.InstallTestClient)
		common.CheckIntegrationInstall(tt, t.InstallTestClient)

		tt.Run("default python version", func(tt *testing.T) {
			pythonVersion, err := t.InstallTestClient.GetPythonVersion()
			if !assert.NoError(tt, err, "should get python version") {
				return
			}
			majorPythonVersion := strings.Split(pythonVersion, ".")[0]

			if t.ExpectPython2Installed() {
				assert.Equal(tt, "2", majorPythonVersion, "Agent 6 should install Python 2")
			} else {
				assert.Equal(tt, "3", majorPythonVersion, "Agent should install Python 3")
			}
		})

		if t.ExpectPython2Installed() {
			tt.Run("switch to Python3", func(tt *testing.T) {
				common.SetAgentPythonMajorVersion(tt, t.InstallTestClient, "3")
				common.CheckAgentPython(tt, t.InstallTestClient, common.ExpectedPythonVersion3)
			})
			tt.Run("switch to Python2", func(tt *testing.T) {
				common.SetAgentPythonMajorVersion(tt, t.InstallTestClient, "2")
				common.CheckAgentPython(tt, t.InstallTestClient, common.ExpectedPythonVersion2)
			})
		}

		if t.ExpectAPM() {
			tt.Run("apm", func(tt *testing.T) {
				common.CheckApmEnabled(tt, t.InstallTestClient)
				common.CheckApmDisabled(tt, t.InstallTestClient)
			})
		}

		if t.ExpectCWS() {
			tt.Run("cws", func(tt *testing.T) {
				common.CheckCWSBehaviour(tt, t.InstallTestClient)
			})
		}
	})
}

// TestUninstallExpectations verifies the agent uninstalled correctly.
func (t *Tester) TestUninstallExpectations(tt *testing.T) {
	tt.Run("", func(tt *testing.T) {
		// this helper uses require so wrap it in a subtest so we can continue even if it fails
		common.CheckUninstallation(tt, t.InstallTestClient)
	})

	_, err := t.host.Lstat(t.expectedInstallPath)
	assert.ErrorIs(tt, err, fs.ErrNotExist, "uninstall should remove install path")
	_, err = t.host.Lstat(t.expectedConfigRoot)
	assert.NoError(tt, err, "uninstall should not remove config root")

	for _, configPath := range getExpectedConfigFiles() {
		configPath := filepath.Join(t.expectedConfigRoot, configPath)
		_, err = t.host.Lstat(configPath)
		assert.NoError(tt, err, "uninstall should not remove %s config file", configPath)
		examplePath := configPath + ".example"
		_, err = t.host.Lstat(examplePath)
		assert.ErrorIs(tt, err, fs.ErrNotExist, "uninstall should remove %s example config files", examplePath)
	}

	_, err = t.host.Lstat(filepath.Join(t.expectedConfigRoot, "auth_token"))
	assert.ErrorIs(tt, err, fs.ErrNotExist, "uninstall should remove auth_token")

	_, err = t.host.Lstat(filepath.Join(t.expectedConfigRoot, "checks.d"))
	assert.ErrorIs(tt, err, fs.ErrNotExist, "uninstall should remove checks.d")

	_, err = windows.GetSIDForUser(t.host,
		windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
	)
	assert.NoError(tt, err, "uninstall should not remove agent user")

	for _, serviceName := range servicetest.ExpectedInstalledServices() {
		conf, err := windows.GetServiceConfig(t.host, serviceName)
		if err == nil && windows.IsKernelModeServiceType(conf.ServiceType) {
			// TODO WKINT-410: kernel mode services are not removed on install rollback
			tt.Logf("WKINT-410: Skipping known failure, kernel mode service not removed: %s", serviceName)
			continue
		}
		assert.Errorf(tt, err, "uninstall should remove service %s", serviceName)
	}

	registryKeyExists, err := windows.RegistryKeyExists(t.host, windowsAgent.RegistryKeyPath)
	assert.NoError(tt, err, "should check registry key exists")
	assert.False(tt, registryKeyExists, "uninstall should remove registry key")
	// don't need to check registry key permissions because the key is removed

	tt.Run("file permissions", func(tt *testing.T) {
		t.testUninstalledFilePermissions(tt)
	})
}

// Only do some basic checks on the agent since it's a previous version
func (t *Tester) testPreviousVersionExpectations(tt *testing.T) {
	RequireAgentRunningWithNoErrors(tt, t.InstallTestClient)
}

// More in depth checks on current version
func (t *Tester) testCurrentVersionExpectations(tt *testing.T) {
	common.CheckInstallation(tt, t.InstallTestClient)

	ddagentUserIdentity, err := windows.GetIdentityForUser(t.host,
		windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
	)
	require.NoError(tt, err)
	everyoneIdentity, err := windows.GetIdentityForSID(t.host, "S-1-1-0")
	require.NoError(tt, err)

	// If install paths differ from default ensure the defaults don't exist
	if t.expectedInstallPath != windowsAgent.DefaultInstallPath {
		_, err := t.host.Lstat(windowsAgent.DefaultInstallPath)
		assert.ErrorIs(tt, err, fs.ErrNotExist, "default install path should not exist")
	}
	if t.expectedConfigRoot != windowsAgent.DefaultConfigRoot {
		_, err := t.host.Lstat(windowsAgent.DefaultConfigRoot)
		assert.ErrorIs(tt, err, fs.ErrNotExist, "default config root should not exist")
	}

	tt.Run("agent paths in registry", func(tt *testing.T) {
		installPathFromRegistry, err := windowsAgent.GetInstallPathFromRegistry(t.host)
		assert.NoError(tt, err, "InstallPath should be in registry")
		assert.Equalf(tt,
			windows.TrimTrailingSlashesAndLower(t.expectedInstallPath),
			windows.TrimTrailingSlashesAndLower(installPathFromRegistry),
			"install path matches registry")
		configRootFromRegistry, err := windowsAgent.GetConfigRootFromRegistry(t.host)
		assert.NoError(tt, err, "ConfigRoot should be in registry")
		assert.Equalf(tt,
			windows.TrimTrailingSlashesAndLower(t.expectedConfigRoot),
			windows.TrimTrailingSlashesAndLower(configRootFromRegistry),
			"config root matches registry")
	})

	tt.Run("agent user in registry", func(tt *testing.T) {
		AssertInstalledUserInRegistry(tt, t.host, t.expectedUserDomain, t.expectedUserName)
	})

	tt.Run("creates config files", func(tt *testing.T) {
		for _, configPath := range getExpectedConfigFiles() {
			configPath := filepath.Join(t.expectedConfigRoot, configPath)
			_, err := t.host.Lstat(configPath)
			assert.NoError(tt, err, "install should create %s config file", configPath)
			examplePath := configPath + ".example"
			_, err = t.host.Lstat(examplePath)
			assert.NoError(tt, err, "install should create %s example config files", examplePath)
		}
	})

	tt.Run("creates bin files", func(tt *testing.T) {
		expected := getExpectedBinFilesForAgentMajorVersion(t.expectedAgentMajorVersion)
		for _, binPath := range expected {
			binPath = filepath.Join(t.expectedInstallPath, binPath)
			_, err := t.host.Lstat(binPath)
			assert.NoError(tt, err, "install should create %s bin file", binPath)
		}
	})

	serviceTester, err := servicetest.NewTester(t.host,
		servicetest.WithExpectedAgentUser(t.expectedUserDomain, t.expectedUserName),
		servicetest.WithExpectedInstallPath(t.expectedInstallPath),
		servicetest.WithExpectedConfigRoot(t.expectedConfigRoot),
	)
	require.NoError(tt, err)
	tt.Run("service config", func(tt *testing.T) {
		actual, err := windows.GetServiceConfigMap(t.host, servicetest.ExpectedInstalledServices())
		require.NoError(tt, err)
		expected, err := serviceTester.ExpectedServiceConfig()
		require.NoError(tt, err)
		servicetest.AssertEqualServiceConfigValues(tt, expected, actual)
		// permissions
		for _, serviceName := range servicetest.ExpectedInstalledServices() {
			conf := actual[serviceName]
			if windows.IsKernelModeServiceType(conf.ServiceType) {
				// we don't modify kernel mode services
				continue
			}
			// ddagentuser should have start/stop/read permissions
			security, err := windows.GetServiceSecurityInfo(t.host, serviceName)
			require.NoError(tt, err)
			expected := windows.NewExplicitAccessRule(
				ddagentUserIdentity,
				windows.SERVICE_START|windows.SERVICE_STOP|windows.SERVICE_GENERIC_READ,
				windows.AccessControlTypeAllow,
			)
			windows.AssertContainsEqualable(tt, security.Access, expected, "%s should have access rule for %s", serviceName, ddagentUserIdentity)
			// [7.47 - 7.50) added an ACE for Everyone, make sure it isn't there
			expected = windows.NewExplicitAccessRule(
				everyoneIdentity,
				windows.SERVICE_ALL_ACCESS,
				windows.AccessControlTypeAllow,
			)
			windows.AssertNotContainsEqualable(tt, security.Access, expected, "%s should not have access rule for %s", serviceName, everyoneIdentity)
		}
	})

	tt.Run("user is a member of expected groups", func(tt *testing.T) {
		AssertAgentUserGroupMembership(tt, t.host,
			windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
		)
	})

	tt.Run("user rights", func(tt *testing.T) {
		AssertUserRights(tt, t.host,
			windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
		)
	})

	tt.Run("file permissions", func(tt *testing.T) {
		t.testInstalledFilePermissions(tt)
	})

	tt.Run("registry permissions", func(tt *testing.T) {
		// ensure registry key has normal inherited permissions and an explicit
		// full access rule for ddagentuser
		agentUserFullAccessDirRule := windows.NewExplicitAccessRule(
			ddagentUserIdentity,
			windows.RegistryFullControl,
			windows.AccessControlTypeAllow,
		)
		path := windowsAgent.RegistryKeyPath
		out, err := windows.GetSecurityInfoForPath(t.host, path)
		require.NoError(tt, err)
		windows.AssertContainsEqualable(tt, out.Access, agentUserFullAccessDirRule, "%s should have full access rule for %s", path, ddagentUserIdentity)
		assert.False(tt, out.AreAccessRulesProtected, "%s should inherit access rules", path)
	})

	RequireAgentRunningWithNoErrors(tt, t.InstallTestClient)

	if !testing.Short() {
		t.runTestsForKitchenCompat(tt)
	}
}

func (t *Tester) testUninstalledFilePermissions(tt *testing.T) {
	// uninstall should remove the agent user from the ACLs
	tc := []struct {
		name             string
		path             string
		expectedSecurity func(t *testing.T) windows.ObjectSecurity
	}{
		{
			name: "ConfigRoot",
			path: t.expectedConfigRoot,
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getBaseConfigRootSecurity(t.host)
				require.NoError(tt, err)
				return s
			},
		},
		{
			name: "datadog.yaml",
			path: filepath.Join(t.expectedConfigRoot, "datadog.yaml"),
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getBaseInheritedConfigFileSecurity(t.host)
				require.NoError(tt, err)
				return s
			},
		},
		{
			name: "conf.d",
			path: filepath.Join(t.expectedConfigRoot, "conf.d"),
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getBaseInheritedConfigDirSecurity(t.host)
				require.NoError(tt, err)
				return s
			},
		},
	}
	for _, tc := range tc {
		tt.Run(tc.name, func(tt *testing.T) {
			out, err := windows.GetSecurityInfoForPath(t.host, tc.path)
			require.NoError(tt, err)
			windows.AssertEqualAccessSecurity(tt, tc.path, tc.expectedSecurity(tt), out)
		})
	}

	// C:\Program Files\Datadog\Datadog Agent (InstallPath)
	// doesn't exist after uninstall so don't need to test
}

func (t *Tester) testInstalledFilePermissions(tt *testing.T) {
	ddagentUserIdentity, err := windows.GetIdentityForUser(t.host,
		windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
	)
	require.NoError(tt, err)

	tc := []struct {
		name             string
		path             string
		expectedSecurity func(t *testing.T) windows.ObjectSecurity
	}{
		{
			name: "ConfigRoot",
			path: t.expectedConfigRoot,
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getExpectedConfigRootSecurityWithAgent(t.host,
					windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
				)
				require.NoError(tt, err)
				return s
			},
		},
		{
			name: "datadog.yaml",
			path: filepath.Join(t.expectedConfigRoot, "datadog.yaml"),
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getExpectedInheritedConfigFileSecurityWithAgent(t.host,
					windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
				)
				require.NoError(tt, err)
				return s
			},
		},
		{
			name: "conf.d",
			path: filepath.Join(t.expectedConfigRoot, "conf.d"),
			expectedSecurity: func(tt *testing.T) windows.ObjectSecurity {
				s, err := getExpectedInheritedConfigDirSecurityWithAgent(t.host,
					windows.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName),
				)
				require.NoError(tt, err)
				return s
			},
		},
	}
	for _, tc := range tc {
		tt.Run(tc.name, func(tt *testing.T) {
			out, err := windows.GetSecurityInfoForPath(t.host, tc.path)
			require.NoError(tt, err)
			windows.AssertEqualAccessSecurity(tt, tc.path, tc.expectedSecurity(tt), out)
		})
	}

	// expect to have standard inherited permissions, plus an explciit ACE for ddagentuser
	embeddedPaths := []string{
		filepath.Join(t.expectedInstallPath, "embedded3"),
	}
	if t.ExpectPython2Installed() {
		embeddedPaths = append(embeddedPaths,
			filepath.Join(t.expectedInstallPath, "embedded2"),
		)
	}
	agentUserFullAccessDirRule := windows.NewExplicitAccessRuleWithFlags(
		ddagentUserIdentity,
		windows.FileFullControl,
		windows.AccessControlTypeAllow,
		windows.InheritanceFlagsContainer|windows.InheritanceFlagsObject,
		windows.PropagationFlagsNone,
	)
	for _, path := range embeddedPaths {
		out, err := windows.GetSecurityInfoForPath(t.host, path)
		require.NoError(tt, err)
		windows.AssertContainsEqualable(tt, out.Access, agentUserFullAccessDirRule, "%s should have full access rule for %s", path, ddagentUserIdentity)
		assert.False(tt, out.AreAccessRulesProtected, "%s should inherit access rules", path)
	}
}

// TestInstallExpectations tests the current agent installation meets the expectations provided to the Tester
func (t *Tester) TestInstallExpectations(tt *testing.T) bool {
	return tt.Run(fmt.Sprintf("test %s", t.agentPackage.AgentVersion()), func(tt *testing.T) {
		if !tt.Run("running expected agent version", func(tt *testing.T) {
			installedVersion, err := t.InstallTestClient.GetAgentVersion()
			require.NoError(tt, err, "should get agent version")
			windowsAgent.TestAgentVersion(tt, t.agentPackage.AgentVersion(), installedVersion)
		}) {
			tt.FailNow()
		}
		if t.isPreviousVersion {
			t.testPreviousVersionExpectations(tt)
		} else {
			t.testCurrentVersionExpectations(tt)
		}
	})
}
