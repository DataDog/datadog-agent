// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	componentsos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

type vmSuite struct {
	e2e.BaseSuite[environments.Host]

	testspath string
}

var (
	devMode = flag.Bool("devmode", false, "run tests in dev mode")
)

func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(componentsos.WindowsDefault))))}
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

	reporoot, _ := filepath.Abs(filepath.Join(currDir, "..", "..", "..", ".."))
	kitchenDir := filepath.Join(reporoot, "test", "kitchen", "site-cookbooks")
	v.testspath = filepath.Join(kitchenDir, "dd-system-probe-check", "files", "default", "tests")
}

func (v *vmSuite) TestSystemProbeSuite() {
	v.BaseSuite.SetupSuite()
	t := v.T()
	// get the remote host
	vm := v.Env().RemoteHost

	err := windows.InstallIIS(vm)
	require.NoError(t, err)
	// HEADSUP the paths are windows, but this will execute in linux. So fix the paths
	t.Log("IIS Installed, continuing")

	t.Log("Creating sites")
	// figure out where we're being executed from.  These paths should be in
	// native path separators (i.e. not windows paths if executing in ci/on linux)

	_, srcfile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	exPath := filepath.Dir(srcfile)

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
	remoteMSIPath, err := windowsCommon.GetTemporaryFile(vm)
	require.NoError(t, err)
	t.Logf("Getting install package %s...", agentPackage.URL)
	err = windowsCommon.PutOrDownloadFile(vm, agentPackage.URL, remoteMSIPath)
	require.NoError(t, err)

	err = windowsCommon.InstallMSI(vm, remoteMSIPath, "", "")
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

	rs.RunTests()
}
