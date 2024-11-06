// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secagentfunctional

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	componentsos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type vmSuite struct {
	e2e.BaseSuite[environments.Host]

	testspath string
}

var (
	devMode = flag.Bool("devmode", false, "run tests in dev mode")
)

func TestVMSuite(t *testing.T) {
	flake.Mark(t)
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

	v.testspath = filepath.Join(currDir, "artifacts")
}

func (v *vmSuite) TestSystemProbeCWSSuite() {
	v.BaseSuite.SetupSuite()
	t := v.T()
	// get the remote host
	vm := v.Env().RemoteHost

	rs := windows.NewRemoteExecutable(vm, t, "testsuite.exe", v.testspath)
	err := rs.FindTestPrograms()
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

	rs.RunTests("5m") // security agent tests can take a while waiting for ETW to start.
}
