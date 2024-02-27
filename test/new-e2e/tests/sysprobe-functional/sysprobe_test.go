// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/iis"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/stretchr/testify/require"
)

type vmSuite struct {
	e2e.BaseSuite[iis.Env]
	//e2e.Suite[iis.Env]
}

var (
	kitchenDir string
	testspath  string
	reporoot   string
	currDir    string
	devMode    = flag.Bool("devmode", false, "run tests in dev mode")
)

func init() {
	// Get the absolute path to the test assets directory
	currDir, _ = os.Getwd()

	reporoot, _ = filepath.Abs(filepath.Join(currDir, "..", "..", "..", ".."))
	kitchenDir = filepath.Join(reporoot, "test", "kitchen", "site-cookbooks")
	testspath = filepath.Join(kitchenDir, "dd-system-probe-check", "files", "default", "tests")
}

func TestVMSuite(t *testing.T) {
	/*
		sites := []windows.IISSiteDefinition{
			{
				Name:        "TestSite1",
				BindingPort: "*:8081:",
				AssetsDir:   path.Join(exPath, "assets"),
			},
			{
				Name:        "TestSite2",
				BindingPort: "*:8082:",
				AssetsDir:   path.Join(exPath, "assets"),
			},
		}

		t.Logf("AssetsDir: %s", sites[0].AssetsDir)
		err = windows.CreateIISSite(vm, sites)
		require.NoError(t, err)
		t.Log("Sites created, continuing")
	*/
	assetsDir := filepath.Join(currDir, "assets")
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(iis.Provisioner(
		iis.WithIISOptions(iis.WithSite("TestSite1", "*:8081:", assetsDir)),
		iis.WithIISOptions(iis.WithSite("TestSite2", "*:8082:", assetsDir)),
	)),
	}

	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestSystemProbeSuite() {
	t := v.T()
	// get the remote host
	vm := v.Env().IISHost

	rs := windows.NewRemoteExecutable(vm, t, "testsuite.exe", testspath)
	err := rs.FindTestPrograms()
	require.NoError(t, err)

	err = rs.CreateRemotePaths()
	require.NoError(t, err)

	err = rs.CopyFiles()
	require.NoError(t, err)

	// install the agent (just so we can get the driver(s) installed)
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	require.NoError(t, err)
	remoteMSIPath, err := common.GetTemporaryFile(vm)
	require.NoError(t, err)
	t.Log("Getting install package...")
	err = common.PutOrDownloadFile(vm, agentPackage.URL, remoteMSIPath)
	require.NoError(t, err)

	err = common.InstallMSI(vm, remoteMSIPath, "", "")
	t.Log("Install complete")
	require.NoError(t, err)

	// disable the agent, and enable the drivers for testing
	_, err = vm.Execute("stop-service -force datadogagent")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config datadogagent start= disabled")
	require.NoError(t, err)
	_, err = vm.Execute("sc.exe config ddnpm start= demand")
	require.NoError(t, err)
	_, err = vm.Execute("start-service ddnpm")
	require.NoError(t, err)

	rs.RunTests()
}
