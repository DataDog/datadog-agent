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
	installUser       string
	isPreviousVersion bool

	// Path to the MSI on the remote host, only available after install is run
	remoteMSIPath string

	expectedUserName   string
	expectedUserDomain string

	expectedAgentVersion      string
	expectedAgentMajorVersion string

	systemFileIntegrityTester *SystemFileIntegrityTester
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

	t.systemFileIntegrityTester = NewSystemFileIntegrityTester(t.host)
	snapshotTaken, err := t.systemFileIntegrityTester.FirstSnapshotTaken()
	require.NoError(tt, err)
	if !snapshotTaken {
		// Only take a snapshot if one doesn't already exist so we can compare across
		// multiple installs.
		err = t.systemFileIntegrityTester.TakeSnapshot()
		require.NoError(tt, err)
	}

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

// WithInstallUser sets the user to install the agent as
func WithInstallUser(user string) TesterOption {
	return func(t *Tester) {
		t.installUser = user
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

func (t *Tester) testDefaultPythonVersion(tt *testing.T) {
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
}

// TestRuntimeExpectations tests the runtime behavior of the agent
func (t *Tester) TestRuntimeExpectations(tt *testing.T) {
	tt.Run("agent runtime behavior", func(tt *testing.T) {
		common.CheckAgentBehaviour(tt, t.InstallTestClient)
		common.CheckAgentStops(tt, t.InstallTestClient)
		common.CheckAgentRestarts(tt, t.InstallTestClient)
		common.CheckIntegrationInstall(tt, t.InstallTestClient)

		t.testDefaultPythonVersion(tt)
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

func (t *Tester) testAgentCodeSignature(tt *testing.T) bool {
	root := `C:\Program Files\Datadog\Datadog Agent\`
	paths := []string{
		// user binaries
		root + `bin\agent.exe`,
		root + `bin\libdatadog-agent-three.dll`,
		root + `bin\agent\trace-agent.exe`,
		root + `bin\agent\process-agent.exe`,
		root + `bin\agent\system-probe.exe`,
		// drivers
		root + `bin\agent\driver\ddnpm.sys`,
	}
	// Python3 should be signed by Python, since we don't build our own anymore
	// We still build our own Python2, so we need to check that
	if t.ExpectPython2Installed() {
		paths = append(paths, []string{
			root + `bin\libdatadog-agent-three.dll`,
			root + `embedded2\python.exe`,
			root + `embedded2\pythonw.exe`,
			root + `embedded2\python27.dll`,
		}...)
	}

	return windowsAgent.TestValidDatadogCodeSignatures(tt, t.host, paths)
}

// TestUninstall uninstalls the agent and runs tests
func (t *Tester) TestUninstall(tt *testing.T, logfile string) bool {
	return tt.Run("uninstall the agent", func(tt *testing.T) {
		if !tt.Run("uninstall", func(tt *testing.T) {
			err := windowsAgent.UninstallAgent(t.host, logfile)
			require.NoError(tt, err, "should uninstall the agent")
		}) {
			tt.Fatal("uninstall failed")
		}

		common.CheckUninstallation(tt, t.InstallTestClient)

		tt.Run("does not change system files", func(tt *testing.T) {
			t.systemFileIntegrityTester.AssertDoesRemoveSystemFiles(tt)
		})
	})
}

func (t *Tester) testRunningExpectedVersion(tt *testing.T) bool {
	return tt.Run("running expected version", func(tt *testing.T) {
		installedVersion, err := t.InstallTestClient.GetAgentVersion()
		require.NoError(tt, err, "should get agent version")
		windowsAgent.TestAgentVersion(tt, t.agentPackage.AgentVersion(), installedVersion)
	})
}

// InstallAgent installs the agent
func (t *Tester) InstallAgent(options ...windowsAgent.InstallAgentOption) error {
	var err error
	opts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithPackage(t.agentPackage),
		windowsAgent.WithValidAPIKey(),
	}
	if t.installUser != "" {
		opts = append(opts, windowsAgent.WithAgentUser(t.installUser))
	}
	opts = append(opts, options...)
	t.remoteMSIPath, err = windowsAgent.InstallAgent(t.host, opts...)
	return err
}

// Only do some basic checks on the agent since it's a previous version
func (t *Tester) testPreviousVersionExpectations(tt *testing.T) {
	common.CheckAgentBehaviour(tt, t.InstallTestClient)
}

// More in depth checks on current version
func (t *Tester) testCurrentVersionExpectations(tt *testing.T) {
	if t.remoteMSIPath != "" {
		windowsAgent.TestValidDatadogCodeSignatures(tt, t.host, []string{t.remoteMSIPath})
	}
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

	t.testAgentCodeSignature(tt)
	t.TestRuntimeExpectations(tt)
}

// TestExpectations tests the current agent installation meets the expectations provided to the Tester
func (t *Tester) TestExpectations(tt *testing.T) bool {
	return tt.Run(fmt.Sprintf("test %s", t.agentPackage.AgentVersion()), func(tt *testing.T) {
		if !t.testRunningExpectedVersion(tt) {
			tt.FailNow()
		}
		if t.isPreviousVersion {
			t.testPreviousVersionExpectations(tt)
		} else {
			t.testCurrentVersionExpectations(tt)
		}
	})
}
