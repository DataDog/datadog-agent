// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/install-test/service-test"

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
}

// TesterOption is a function that can be used to configure a Tester
type TesterOption func(*Tester)

// NewTester creates a new Tester
func NewTester(tt *testing.T, host *components.RemoteHost, opts ...TesterOption) (*Tester, error) {
	t := &Tester{}

	var err error

	t.host = host
	t.InstallTestClient = common.NewWindowsTestClient(tt, t.host)
	t.hostInfo, err = windows.GetHostInfo(t.host)
	if err != nil {
		return nil, err
	}
	t.expectedUserName = "ddagentuser"
	t.expectedUserDomain = windows.NameToNetBIOSName(t.hostInfo.Hostname)

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
}

// Only do some basic checks on the agent since it's a previous version
func (t *Tester) testPreviousVersionExpectations(tt *testing.T) {
	RequireAgentRunningWithNoErrors(tt, t.InstallTestClient)
}

// More in depth checks on current version
func (t *Tester) testCurrentVersionExpectations(tt *testing.T) {
	common.CheckInstallation(tt, t.InstallTestClient)
	tt.Run("user in registry", func(tt *testing.T) {
		AssertInstalledUserInRegistry(tt, t.host, t.expectedUserDomain, t.expectedUserName)
	})

	serviceTester, err := servicetest.NewTester(t.host,
		servicetest.WithExpectedAgentUser(t.expectedUserDomain, t.expectedUserName),
	)
	require.NoError(tt, err)
	serviceTester.TestInstall(tt)

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

	RequireAgentRunningWithNoErrors(tt, t.InstallTestClient)

	t.runTestsForKitchenCompat(tt)
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
