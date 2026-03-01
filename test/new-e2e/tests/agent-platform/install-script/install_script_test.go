// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package installscript

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osDescriptors             = flag.String("osdescriptors", "", "os versions to test")
	cwsSupportedOsDescriptors = flag.String("cws-supported-osdescriptors", "", "list of os descriptors where CWS is supported")
	flavor                    = flag.String("flavor", "datadog-agent", "flavor to test (datadog-agent, datadog-iot-agent, datadog-dogstatsd, datadog-fips-agent, datadog-fips-proxy, datadog-heroku-agent)")
	majorVersion              = flag.String("major-version", "7", "major version to test (6, 7)")
)

type installScriptSuite struct {
	e2e.BaseSuite[environments.Host]
	cwsSupported   bool
	testingKeysURL string
	osDesc         e2eos.Descriptor
}

func TestInstallScript(t *testing.T) {
	if strings.Contains(*osDescriptors, "docker") {
		DockerTest(t)
		return
	}

	osDescriptors, err := platforms.ParseOSDescriptors(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to parse os descriptors: %v", err)
	}
	if len(osDescriptors) == 0 {
		t.Fatal("expecting some value to be passed for --osdescriptors on test invocation, got none")
	}

	cwsSupportedOsVersionList, err := platforms.ParseOSDescriptors(*cwsSupportedOsDescriptors)
	if err != nil {
		t.Fatalf("failed to parse cws supported os version: %v", err)
	}
	for _, osDesc := range osDescriptors {
		osDesc := osDesc

		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osDesc {
				cwsSupported = true
			}
		}

		vmOpts := []ec2.VMOption{}
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

		t.Run(fmt.Sprintf("test install script on %s %s agent %s", platforms.PrettifyOsDescriptor(osDesc), *flavor, *majorVersion), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", platforms.PrettifyOsDescriptor(osDesc))
			vmOpts = append(vmOpts, ec2.WithOS(osDesc))

			suite := &installScriptSuite{cwsSupported: cwsSupported, osDesc: osDesc}
			// will be set as TESTING_KEYS_URL in the install script
			// the used in places like https://github.com/DataDog/agent-linux-install-script/blob/8f5c0b4f5b60847ee7989aa2c35052382f282d5d/install_script.sh.template#L1229
			suite.testingKeysURL = "apttesting.datad0g.com/test-keys"

			e2e.Run(tt,
				suite,
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithRunOptions(ec2.WithEC2InstanceOptions(vmOpts...)),
				)),
				e2e.WithStackName(fmt.Sprintf("install-script-test-%v-%s-%v", platforms.PrettifyOsDescriptor(osDesc), *flavor, *majorVersion)),
			)
		})
	}
}

func DockerTest(t *testing.T) {
	platform, architecture, _, err := platforms.ParseRawOsDescriptor(*osDescriptors)
	if err != nil {
		t.Fatalf("failed to parse os descriptors: %v", err)
	}
	if platform != "docker" {
		t.Fatalf("expected platform to be docker, got %s", platform)
	}

	suite := &installScriptSuiteSysVInit{arch: e2eos.ArchitectureFromString(architecture)}
	suite.testingKeysURL = "apttesting.datad0g.com/test-keys"
	t.Run("test install script on a docker container (using SysVInit)", func(tt *testing.T) {
		e2e.Run(tt,
			suite,
			e2e.WithProvisioner(
				awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithRunOptions(ec2.WithDocker()),
				),
			),
		)
	})
}

func (is *installScriptSuite) TestInstallAgent() {
	switch *flavor {
	case "datadog-agent":
		is.AgentTest("datadog-agent")
	case "datadog-heroku-agent":
		is.AgentTest("datadog-heroku-agent")
	case "datadog-iot-agent":
		is.IotAgentTest()
	case "datadog-dogstatsd":
		is.DogstatsdAgentTest()
	case "datadog-fips-agent":
		is.AgentTest("datadog-fips-agent")
	}
}

func (is *installScriptSuite) testUninstall(client *common.TestClient, flavor string) {
	is.T().Run("remove the agent", func(tt *testing.T) {
		_, err := client.PkgManager.Remove(flavor)
		require.NoError(tt, err, "should uninstall the agent")
	})

	common.CheckUninstallation(is.T(), client)
}

func (is *installScriptSuite) AgentTest(flavor string) {
	host := is.Env().RemoteHost
	fileManager := filemanager.NewUnix(host)
	agentClient, err := client.NewHostAgentClient(is, host.HostOutput, false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnix()
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	installOptions := []installparams.Option{
		installparams.WithArch(string(is.osDesc.Architecture)),
		installparams.WithFlavor(flavor),
		installparams.WithMajorVersion(*majorVersion),
	}

	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), client, installOptions...)

	common.CheckInstallation(is.T(), client)
	common.CheckSigningKeys(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)
	common.CheckIntegrationInstall(is.T(), client)
	if *majorVersion == "6" {
		common.SetAgentPythonMajorVersion(is.T(), client, "2")
		common.CheckAgentPython(is.T(), client, common.ExpectedPythonVersion2)
	}
	common.SetAgentPythonMajorVersion(is.T(), client, "3")
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckAgentPython(is.T(), client, common.ExpectedPythonVersion3)
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckApmEnabled(is.T(), client)
	time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
	common.CheckApmDisabled(is.T(), client)
	if flavor == "datadog-agent" {
		common.CheckSystemProbeBehavior(is.T(), client)
		if is.cwsSupported {
			common.CheckCWSBehaviour(is.T(), client)
		}

		// time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
		// common.CheckADPEnabled(is.T(), client)
		// time.Sleep(5 * time.Second) // Restarting the agent too fast will cause systemctl to fail
		// common.CheckADPDisabled(is.T(), client)
	}

	common.CheckInstallationInstallScript(is.T(), client)
	is.testUninstall(client, flavor)
}

func (is *installScriptSuite) IotAgentTest() {
	host := is.Env().RemoteHost
	fileManager := filemanager.NewUnix(host)
	agentClient, err := client.NewHostAgentClient(is, host.HostOutput, false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnix()
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	installOptions := []installparams.Option{
		installparams.WithArch(string(is.osDesc.Architecture)),
		installparams.WithFlavor(*flavor),
	}

	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), client, installOptions...)

	common.CheckInstallation(is.T(), client)
	common.CheckSigningKeys(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)

	common.CheckInstallationInstallScript(is.T(), client)
	is.testUninstall(client, "datadog-iot-agent")
}

func (is *installScriptSuite) DogstatsdAgentTest() {
	host := is.Env().RemoteHost
	fileManager := filemanager.NewUnix(host)
	agentClient, err := client.NewHostAgentClient(is, host.HostOutput, false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnixDogstatsd()
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	installOptions := []installparams.Option{
		installparams.WithArch(string(is.osDesc.Architecture)),
		installparams.WithFlavor(*flavor),
	}

	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), client, installOptions...)

	common.CheckInstallation(is.T(), client)
	common.CheckSigningKeys(is.T(), client)
	common.CheckDogstatdAgentBehaviour(is.T(), client)
	common.CheckDogstatsdAgentStops(is.T(), client)
	common.CheckDogstatsdAgentRestarts(is.T(), client)
	common.CheckInstallationInstallScript(is.T(), client)
	is.testUninstall(client, "datadog-dogstatsd")
}

type installScriptSuiteSysVInit struct {
	e2e.BaseSuite[environments.Host]
	arch           e2eos.Architecture
	testingKeysURL string
}

func (is *installScriptSuiteSysVInit) TestInstallAgent() {
	containerName := "installation-target"
	host := is.Env().RemoteHost
	client := common.NewDockerTestClient(host, containerName)

	err := client.RunContainer("public.ecr.aws/ubuntu/ubuntu:22.04_stable")
	require.NoError(is.T(), err)
	defer client.Cleanup()

	// We need `curl` to download the script, and `sudo` because all the existing test helpers run commands
	// through it.
	_, err = client.ExecuteWithRetry("apt-get update && apt-get install -y curl sudo")
	require.NoError(is.T(), err)

	installOptions := []installparams.Option{
		installparams.WithArch(string(is.arch)),
		installparams.WithFlavor(*flavor),
	}

	if is.testingKeysURL != "" {
		installOptions = append(installOptions, installparams.WithTestingKeysURL(is.testingKeysURL))
	}
	install.Unix(is.T(), client, installOptions...)

	// We can't easily reuse the the helpers that assume everything runs directly on the host
	// We run a few selected sanity checks here instead (sufficient for this platform anyway)
	is.T().Run("datadog-agent service running", func(tt *testing.T) {
		_, err := client.Execute("service datadog-agent status")
		require.NoError(tt, err, "datadog-agent service should be running")
	})
	is.T().Run("status command no errors", func(tt *testing.T) {
		statusOutput, err := client.ExecuteWithRetry("datadog-agent \"status\"")
		require.NoError(is.T(), err)

		// API Key is invalid we should not check for the following error
		statusOutput = strings.ReplaceAll(statusOutput, "[ERROR] API Key is invalid", "API Key is invalid")
		require.NotContains(tt, statusOutput, "ERROR")
	})
}
