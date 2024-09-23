// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package installscript

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

var (
	osVersion             = flag.String("osversion", "", "os version to test")
	platform              = flag.String("platform", "", "platform to test")
	cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")
	architecture          = flag.String("arch", "x86_64", "architecture to test (x86_64, arm64))")
	flavor                = flag.String("flavor", "datadog-agent", "flavor to test (datadog-agent, datadog-iot-agent, datadog-dogstatsd, datadog-fips-proxy, datadog-heroku-agent)")
	majorVersion          = flag.String("major-version", "7", "major version to test (6, 7)")
)

type installScriptSuite struct {
	e2e.BaseSuite[environments.Host]
	cwsSupported bool
}

func TestInstallScript(t *testing.T) {
	if *platform == "docker" {
		DockerTest(t)
		return
	}

	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	// Splitting an empty string results in a slice with a single empty string which wouldn't be useful
	// and result in no tests being run; let's fail the test to make it obvious
	if strings.TrimFunc(*osVersion, unicode.IsSpace) == "" {
		t.Fatal("expecting some value to be passed for --osversion on test invocation, got none")
	}
	osVersions := strings.Split(*osVersion, ",")
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")

	t.Log("Parsed platform json file: ", platformJSON)

	for _, osVers := range osVersions {
		osVers := osVers
		if platformJSON[*platform][*architecture][osVers] == "" {
			// Fail if the image is not defined instead of silently running with default Ubuntu AMI
			t.Fatalf("No image found for %s %s %s", *platform, *architecture, osVers)
		}

		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		vmOpts := []ec2.VMOption{}
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

		t.Run(fmt.Sprintf("test install script on %s %s %s agent %s", osVers, *architecture, *flavor, *majorVersion), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", osVers)
			osDesc := platforms.BuildOSDescriptor(*platform, *architecture, osVers)
			vmOpts = append(vmOpts, ec2.WithAMI(platformJSON[*platform][*architecture][osVers], osDesc, osDesc.Architecture))

			e2e.Run(tt,
				&installScriptSuite{cwsSupported: cwsSupported},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithEC2InstanceOptions(vmOpts...),
				)),
				e2e.WithStackName(fmt.Sprintf("install-script-test-%v-%s-%s-%v", osVers, *architecture, *flavor, *majorVersion)),
			)
		})
	}

}

func DockerTest(t *testing.T) {
	t.Run("test install script on a docker container (using SysVInit)", func(tt *testing.T) {
		e2e.Run(tt,
			&installScriptSuiteSysVInit{},
			e2e.WithProvisioner(
				awshost.ProvisionerNoAgentNoFakeIntake(
					awshost.WithDocker(),
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

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(flavor), installparams.WithMajorVersion(*majorVersion))

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
	common.CheckAgentPython(is.T(), client, common.ExpectedPythonVersion3)
	common.CheckApmEnabled(is.T(), client)
	common.CheckApmDisabled(is.T(), client)
	if flavor == "datadog-agent" {
		common.CheckSystemProbeBehavior(is.T(), client)
		if is.cwsSupported {
			common.CheckCWSBehaviour(is.T(), client)
		}
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

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

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

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

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

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

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
