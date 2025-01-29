// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/resources/aws"

	infraos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"

	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
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
	e2e.BaseSuite[multiVMEnv]
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

	// Enable FIPS mode
	err := windowsCommon.EnableFIPSMode(s.Env().WindowsVM)
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
	logFile := filepath.Join(s.SessionOutputDir(), "install.log")
	_, err = windowsAgent.InstallAgent(s.Env().WindowsVM,
		windowsAgent.WithPackage(agentPackage),
		windowsAgent.WithInstallLogFile(logFile))
	require.NoError(s.T(), err)
}

func (s *fipsServerWinSuite) fipsServerUp(tc cipherTestCase) {
	// Write docker-compose.yaml to disk
	host := s.Env().LinuxDockerVM
	_, err := host.WriteFile("/tmp/docker-compose.yaml", dockerFipsServerCompose)
	require.NoError(s.T(), err)

	// stop currently running server, so we can reset logs+env
	s.fipsServerDown()

	// start datadog/appd-fips-server with env vars from the test case
	envVars := map[string]string{
		"CERT": tc.cert,
	}
	if tc.cipher != "" {
		envVars["CIPHER"] = fmt.Sprintf("-c %s", tc.cipher)
	}
	if tc.tlsMax != "" {
		envVars["TLS_MAX"] = fmt.Sprintf("--tls-max %s", tc.tlsMax)
	}

	cmd := "docker-compose -f /tmp/docker-compose.yaml up --detach --wait --timeout 300"
	_, err = host.Execute(cmd, client.WithEnvVariables(envVars))
	require.NoError(s.T(), err)

	// Wait for container to start and ensure it's a fresh instance
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		serverLogs, _ := host.Execute("docker logs dd-fips-server")
		assert.Contains(t, serverLogs, "Server Starting...", "Server should start")
		assert.Equal(t, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 60*time.Second, 5*time.Second)
}

func (s *fipsServerWinSuite) fipsServerDown() {
	host := s.Env().LinuxDockerVM
	_, err := host.Execute("docker-compose -f /tmp/docker-compose.yaml down")
	require.NoError(s.T(), err)
}

func (s *fipsServerWinSuite) TestFIPSCiphers() {
	host := s.Env().WindowsVM
	dockerHost := s.Env().LinuxDockerVM

	for _, tc := range testcases {
		s.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {
			// Start the fips-server and waits for it to be ready
			s.fipsServerUp(tc)
			s.T().Cleanup(func() {
				s.fipsServerDown()
			})

			agentEnv := map[string]string{
				// datadog/apps-fips-server creates self-signed certed
				"DD_SKIP_SSL_VALIDATION": "true",
				// point diagnose command at datadog/apps-fips-server container
				"DD_DD_URL": fmt.Sprintf(`https://%s:443`, dockerHost.HostOutput.Address),
			}
			agent := `C:\Program Files\Datadog\Datadog Agent\bin\agent.exe`
			cmd := fmt.Sprintf(`& '%s' diagnose --include connectivity-datadog-core-endpoints --local`, agent)
			host.Execute(cmd, client.WithEnvVariables(agentEnv))

			serverLogs := dockerHost.MustExecute("docker logs dd-fips-server")
			if tc.want {
				assert.Contains(s.T(), serverLogs, fmt.Sprintf("Negotiated cipher suite: %s", tc.cipher))
			} else {
				assert.Contains(s.T(), serverLogs, "no cipher suite supported by both client and server")
			}
		})
	}
}

// multiVMEnvProvisioner provisions a Windows VM and a Linux Docker VM
//
// This allows us to run the datadog/apps-fips-server container on the Linux VM,
// and query it from the Agent running on the Windows VM.
//
// Why do we do it this way?
//   - The fips_ciphers_nix_test.go uses test-infra DockerHost which doesn't support Windows.
//   - The fips-server is a go binary, so it might be portable to Windows, but TBD.
//   - The datadog/apps-fips-server container is only built for Linux, but Windows
//     Server / Docker EE do NOT support Linux containers.
//   - AWS only supports nested virtualization on metal instances, so that's a hurdle to using WSL2,
//     plus would also have to do the WSL2 and docker setup manually.
//   - E2E environments/provisioners are not flexible, so we must create our own to customize the env
//   - E2E environments/provisioners are not reusable, so we have to copy/paste the DockerHost setup
func multiVMEnvProvisioner() provisioners.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		windowsVM, err := ec2.NewVM(awsEnv, "WindowsVM", ec2.WithOS(infraos.WindowsDefault))
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

	// install the ECR credentials helper
	// required to get pipeline agent images
	// TODO: cred helper might not be needed? not sure about auth on fips-server image
	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, linuxDockerVM)
	if err != nil {
		return err
	}
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
