// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windows "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

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

	// Path to the MSI on the remote host, only available after install is run
	remoteMSIPath string

	expectedAgentVersion      string
	expectedAgentMajorVersion string

	beforeInstallSystemDirListPath  string
	afterUninstallSystemDirListPath string
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

	t.beforeInstallSystemDirListPath = `C:\system-files-before-install.log`
	t.afterUninstallSystemDirListPath = `C:\system-files-after-uninstall.log`

	// If the system file snapshot doesn't exist, create it
	snapshotExists, err := t.host.FileExists(t.beforeInstallSystemDirListPath)
	if err != nil {
		return nil, err
	}
	if !snapshotExists {
		if !tt.Run("snapshot system files", func(tt *testing.T) {
			err = t.snapshotSystemfiles(tt, t.beforeInstallSystemDirListPath)
			require.NoError(tt, err)
		}) {
			tt.FailNow()
		}
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

func (t *Tester) snapshotSystemfiles(tt *testing.T, remotePath string) error {
	// Ignore these paths when collecting the list of files, they are known to frequently change
	// Ignoring paths while creating the snapshot reduces the snapshot size by >90%
	ignorePaths := []string{
		`C:\Windows\Assembly\Temp\`,
		`C:\Windows\Assembly\Tmp\`,
		`C:\windows\AppReadiness\`,
		`C:\Windows\Temp\`,
		`C:\Windows\Prefetch\`,
		`C:\Windows\Installer\`,
		`C:\Windows\WinSxS\`,
		`C:\Windows\Logs\`,
		`C:\Windows\servicing\`,
		`c:\Windows\System32\catroot2\`,
		`c:\windows\System32\config\`,
		`c:\windows\System32\sru\`,
		`C:\Windows\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Logs\`,
		`C:\Windows\ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache\`,
		`C:\Windows\SoftwareDistribution\DataStore\Logs\`,
		`C:\Windows\System32\wbem\Performance\`,
		`c:\windows\System32\LogFiles\`,
		`c:\windows\SoftwareDistribution\`,
		`c:\windows\ServiceProfiles\NetworkService\AppData\`,
		`c:\windows\System32\Tasks\Microsoft\Windows\UpdateOrchestrator\`,
		`c:\windows\System32\Tasks\Microsoft\Windows\Windows Defender\Windows Defender Scheduled Scan`,
		`C:\Windows\System32\Tasks\MicrosoftEdgeUpdateBrowserReplacementTask`,
	}
	// quote each path and join with commas
	pattern := ""
	for _, ignorePath := range ignorePaths {
		pattern += fmt.Sprintf(`'%s',`, ignorePath)
	}
	// PowerShell list syntax
	pattern = fmt.Sprintf(`@(%s)`, strings.Trim(pattern, ","))
	// Recursively list Windows directory and ignore the paths above
	// Compare-Object is case insensitive by default
	cmd := fmt.Sprintf(`cmd /c dir C:\Windows /b /s | Out-String -Stream | Select-String -NotMatch -SimpleMatch -Pattern %s | Select -ExpandProperty Line > "%s"`, pattern, remotePath)
	require.Less(tt, len(cmd), 8192, "should not exceed max command length")
	_, err := t.host.Execute(cmd)
	require.NoError(tt, err, "should snapshot system files")
	// sanity check to ensure file contains a reasonable amount of output
	stat, err := t.host.Lstat(remotePath)
	require.Greater(tt, stat.Size(), int64(1024*1024), "system file snapshot should be at least 1MB")
	return err
}

func (t *Tester) testDoesNotChangeSystemFiles(tt *testing.T) bool {
	return tt.Run("does not remove system files", func(tt *testing.T) {
		tt.Cleanup(func() {
			// Remove the snapshot files after the test
			err := t.host.Remove(t.beforeInstallSystemDirListPath)
			if err != nil {
				tt.Logf("failed to remove %s: %s", t.beforeInstallSystemDirListPath, err)
			}
			err = t.host.Remove(t.afterUninstallSystemDirListPath)
			if err != nil {
				tt.Logf("failed to remove %s: %s", t.afterUninstallSystemDirListPath, err)
			}
		})
		// Diff the two files on the remote host, selecting missing items
		cmd := fmt.Sprintf(`Compare-Object -ReferenceObject (Get-Content "%s") -DifferenceObject (Get-Content "%s") | Where-Object -Property SideIndicator -EQ '<=' | Select -ExpandProperty InputObject`, t.beforeInstallSystemDirListPath, t.afterUninstallSystemDirListPath)
		output, err := t.host.Execute(cmd)
		require.NoError(tt, err, "should compare system files")
		output = strings.TrimSpace(output)
		if output != "" {
			// Log result since flake.Mark may skip the test before the assertion is run
			tt.Logf("should not remove system files: %s", output)
			// Since the result of this test can depend on Windows behavior unrelated to the agent,
			// we mark it as flaky so it doesn't block PRs.
			// See WINA-624 for investigation into better ways to perform this test.
			// If new Windows paths must be ignored, add them to the ignorePaths list in snapshotSystemfiles.
			flake.Mark(tt)
			// Skipping does not remove the failed test status, so we must run the assertion after flake.Mark.
			require.Empty(tt, output, "should not remove system files")
		}
	})
}

// InstallAgentPackage installs the agent and returns any errors
func (t *Tester) InstallAgentPackage(tt *testing.T, agentPackage *windowsAgent.Package, args string, logfile string) (string, error) {
	// Put the MSI on the host
	remoteMSIPath, err := windows.GetTemporaryFile(t.host)
	require.NoError(tt, err)
	err = windows.PutOrDownloadFile(t.host, agentPackage.URL, remoteMSIPath)
	require.NoError(tt, err)

	if !strings.Contains(args, "APIKEY") {
		// TODO: Add apikey option
		apikey := "00000000000000000000000000000000"
		args = fmt.Sprintf(`%s APIKEY="%s"`, args, apikey)
	}
	err = windows.InstallMSI(t.host, remoteMSIPath, args, logfile)
	return remoteMSIPath, err
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

		if !tt.Run("snapshot system files", func(tt *testing.T) {
			err := t.snapshotSystemfiles(tt, t.afterUninstallSystemDirListPath)
			require.NoError(tt, err)
		}) {
			tt.Fatal("snapshot system files failed")
		}

		t.testDoesNotChangeSystemFiles(tt)
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
func (t *Tester) InstallAgent(tt *testing.T, args string, logfile string) error {
	var err error
	t.remoteMSIPath, err = t.InstallAgentPackage(tt, t.agentPackage, args, logfile)
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
