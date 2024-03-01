// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobefunctional

import (
	"flag"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/iis"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	componentsos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

type sysprobeEnv struct {
	environments.Host
	IISServer *iis.Output
}

type vmSuite struct {
	windows.BaseAgentInstallerSuite[sysprobeEnv]
}

var (
	kitchenDir string
	testspath  string
	reporoot   string
	devMode    = flag.Bool("devmode", false, "run tests in dev mode")
)

func init() {
	// Get the absolute path to the test assets directory
	currDir, _ := os.Getwd()

	reporoot, _ = filepath.Abs(filepath.Join(currDir, "..", "..", "..", ".."))
	kitchenDir = filepath.Join(reporoot, "test", "kitchen", "site-cookbooks")
	testspath = filepath.Join(kitchenDir, "dd-system-probe-check", "files", "default", "tests")
}

func TestVMSuite(t *testing.T) {
	_, srcfile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	exPath := filepath.Dir(srcfile)
	srcDir := path.Join(exPath, "assets")

	provisioner := e2e.NewTypedPulumiProvisioner("aws-ec2-sysprobe", func(ctx *pulumi.Context, env *sysprobeEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		vm, err := ec2.NewVM(awsEnv, "vm", ec2.WithOS(componentsos.WindowsDefault))
		if err != nil {
			return err
		}
		err = vm.Export(ctx, &env.RemoteHost.HostOutput)
		if err != nil {
			return err
		}
		iisServer, err := iis.NewIISServer(ctx, awsEnv.CommonEnvironment, vm,
			iis.WithSite(iis.IISSiteDefinition{
				Name:            "TestSite1",
				BindingPort:     "*:8081:",
				SourceAssetsDir: srcDir,
			}), iis.WithSite(iis.IISSiteDefinition{
				Name:            "TestSite2",
				BindingPort:     "*:8081:",
				SourceAssetsDir: srcDir,
			}),
		)
		if err != nil {
			return err
		}
		err = iisServer.Export(ctx, env.IISServer)
		if err != nil {
			return err
		}
		return err
	}, nil)

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioner)}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestSystemProbeSuite() {
	t := v.T()
	// get the remote host
	vm := v.Env().RemoteHost

	var err error

	rs := windows.NewRemoteExecutable(vm, t, "testsuite.exe", testspath)
	err = rs.FindTestPrograms()
	require.NoError(t, err)

	err = rs.CreateRemotePaths()
	require.NoError(t, err)

	err = rs.CopyFiles()
	require.NoError(t, err)

	_, err = v.InstallAgent(vm,
		windowsAgent.WithPackage(v.AgentPackage),
		windowsAgent.WithInstallLogFile("install.log"))

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
