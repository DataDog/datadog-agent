// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	infraos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/docker-compose-fips-server.yaml
var dockerFipsServerCompose []byte

type multiVMEnv struct {
	WindowsVM     *components.RemoteHost
	LinuxDockerVM *components.RemoteHost
	LinuxDocker   *components.RemoteHostDocker
}

type fipsServerWinSuite struct {
	fipsServerSuite[multiVMEnv]
}

func TestFIPSCiphersWindowsSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithPulumiProvisioner(multiVMEnvProvisioner(), nil)}

	e2e.Run(
		t,
		&fipsServerWinSuite{},
		suiteParams...,
	)
}

func (s *fipsServerWinSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	agentHost := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM

	// supply workers to base fipsServerSuite
	composeFilePath := "/tmp/docker-compose.yaml"
	s.fipsServer = newFIPSServer(dockerHost, composeFilePath)
	s.generateTestTraffic = s.generateTraffic

	// Write docker-compose.yaml to disk
	_, err := dockerHost.WriteFile(composeFilePath, bytes.ReplaceAll(dockerFipsServerCompose, []byte("{APPS_VERSION}"), []byte(apps.Version)))
	require.NoError(s.T(), err)

	// Enable FIPS mode for OS
	err = windowsCommon.EnableFIPSMode(agentHost)
	require.NoError(s.T(), err)

	// Install FIPS Agent
	// NOTE: Installing the Agent manually instead of using the Agent component
	// to make devloops easier, as the Agent component does not support
	// installing locally built packages.
	if _, set := windowsAgent.LookupFlavorFromEnv(); !set {
		os.Setenv(windowsAgent.PackageFlavorEnvVar, "fips")
	}
	agentPackage, err := windowsAgent.GetPackageFromEnv()
	require.NoError(s.T(), err)
	s.T().Logf("Using Agent: %#v", agentPackage)
	logFile := filepath.Join(s.SessionOutputDir(), "install.log")
	_, err = windowsAgent.InstallAgent(agentHost,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithInstallLogFile(logFile),
		// The connectivity-datadog-core-endpoints diagnoses require a non-empty API key
		windowsAgent.WithZeroAPIKey())
	require.NoError(s.T(), err)

	// Start the fips-server in the Setup step so we pull the image from ghcr.io before a test runs
	s.fipsServer.Start(s.T(), cipherTestCase{cert: "rsa"})
}

func (s *fipsServerWinSuite) generateTraffic() {
	agentHost := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM
	agentEnv := map[string]string{
		// datadog/apps-fips-server creates self-signed cert
		"DD_SKIP_SSL_VALIDATION": "true",
		// point diagnose command at datadog/apps-fips-server container
		"DD_DD_URL": fmt.Sprintf(`https://%s:443`, dockerHost.HostOutput.Address),
	}
	agent := `C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`
	cmd := fmt.Sprintf(`& '%s' diagnose --include connectivity-datadog-core-endpoints --local`, agent)
	out, _ := agentHost.Execute(cmd, client.WithEnvVariables(agentEnv))
	require.NotContains(s.T(), out, "Total:0", "Expected diagnoses to run, ensure an API key is configured")
}

// multiVMEnvProvisioner provisions a Windows VM and a Linux Docker VM
//
// This allows us to run the datadog/apps-fips-server container on the Linux VM,
// and query it from the Agent running on the Windows VM. This is different from
// the Linux test which runs the Agent and fips-server on the same host with environments.DockerHost.
// This means the DockerHost provisioner args are not usable in this test, and this test isn't using any
// special params right now so we didn't copy over the param code.
//
// Why use a custom provisioner?
//   - fips_ciphers_nix_test.go uses test-infra DockerHost which doesn't support Windows.
//   - E2E environments/provisioners are not flexible/combineable, so we must create one from scratch to customize the env
//
// Can fips-server run on Windows?
//   - The datadog/apps-fips-server container is only built for Linux
//   - it's a go binary, so it might be portable to Windows / Windows container
//
// Can we run the Linux container on Windows?
//   - Windows Server / Docker EE do not support Linux containers
//   - AWS only has built-in Windows Server AMIs, not clients, so its more effort to use
//     a custom AMI and then install Docker Desktop to get Linux container support.
//   - AWS only supports nested virtualization on metal instances, so that's a hurdle to using WSL2,
//     plus would also have to do the WSL2 and docker setup manually.
//
// Long term vision / ideal scenario:
//   - port fips-server to Windows and create a Windows container image
//   - Add Windows support to test-infra DockerHost
//   - Use DockerHost env in this test
func multiVMEnvProvisioner() provisioners.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		windowsVM, err := ec2.NewVM(awsEnv, "WindowsVM", ec2.WithOS(infraos.WindowsServerDefault))
		if err != nil {
			return err
		}
		windowsVM.Export(ctx, &env.WindowsVM.HostOutput)

		err = linuxDockerVMProvisioner(ctx, awsEnv, env)
		if err != nil {
			return err
		}

		return nil
	}
}

func linuxDockerVMProvisioner(ctx *pulumi.Context, awsEnv aws.Environment, env *multiVMEnv) error {
	linuxDockerVM, err := ec2.NewVM(awsEnv, "LinuxDockerVM", ec2.WithOS(infraos.UbuntuDefault))
	if err != nil {
		return err
	}
	linuxDockerVM.Export(ctx, &env.LinuxDockerVM.HostOutput)

	// Install+Configure Docker on LinuxDockerVM
	//
	// copied from docker environment/provisioner
	//

	manager, err := docker.NewManager(&awsEnv, linuxDockerVM, utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}
	err = manager.Export(ctx, &env.LinuxDocker.ManagerOutput)
	if err != nil {
		return err
	}
	//
	// end copied from docker environment/provisioner
	//

	return nil
}
