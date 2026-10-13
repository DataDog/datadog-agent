// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type vmSuite struct {
	e2e.BaseSuite[environments.WindowsHost]

	testspath string
}

var (
	devMode = flag.Bool("devmode", false, "run tests in dev mode")
)

func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake())}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) SetupSuite() {
	t := v.T()

	// Get the absolute path to the test assets directory
	currDir, err := os.Getwd()
	require.NoError(t, err)

	v.testspath = filepath.Join(currDir, "artifacts")
}

func (v *vmSuite) TestSystemProbeNPMSuite() {
	v.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer v.CleanupOnSetupFailure()

	t := v.T()
	// get the remote host
	vm := v.Env().RemoteHost

	err := windows.InstallIIS(vm)
	require.NoError(t, err)
	// HEADSUP the paths are windows, but this will execute in linux. So fix the paths
	t.Log("IIS Installed, continuing")

	t.Log("Creating sites")
	assetsDir, err := fs.Sub(embeddedAssets, "assets")
	require.NoError(t, err)
	sites := []windows.IISSiteDefinition{
		{
			Name:        "TestSite1",
			BindingPort: "*:8081:",
			AssetsDir:   assetsDir,
		},
		{
			Name:        "TestSite2",
			BindingPort: "*:8082:",
			AssetsDir:   assetsDir,
		},
	}

	err = windows.CreateIISSite(vm, sites)
	require.NoError(t, err)
	t.Log("Sites created, continuing")

	rs := windows.NewRemoteExecutable(vm, t, "testsuite.exe", v.testspath)
	err = rs.FindTestPrograms()
	require.NoError(t, err)

	err = rs.CreateRemotePaths()
	require.NoError(t, err)

	err = rs.CopyFiles()
	require.NoError(t, err)

	// install the agent (just so we can get the driver(s) installed)
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	require.NoError(t, err)
	_, err = windowsAgent.InstallAgent(vm, windowsAgent.WithPackage(agentPackage))
	t.Log("Install complete")
	require.NoError(t, err)

	time.Sleep(30 * time.Second)
	// disable the agent, and enable the drivers for testing
	_, err = vm.Execute("stop-service -force datadogagent")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config datadogagent start= disabled")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config ddnpm start= demand")
	require.NoError(t, err)
	_, err = vm.Execute("start-service ddnpm")
	require.NoError(t, err)

	rs.RunTests("")
}
