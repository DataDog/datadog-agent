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
	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

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
				e2e.WithStackName(fmt.Sprintf("install-script-test-%v-%v-%s-%s-%v", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *flavor, *majorVersion)),
			)
		})
	}
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
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
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
		common.CheckAgentPython(is.T(), client, "2")
	}
	common.CheckAgentPython(is.T(), client, "3")
	common.CheckApmEnabled(is.T(), client)
	common.CheckApmDisabled(is.T(), client)
	if flavor == "datadog-agent" && is.cwsSupported {
		common.CheckCWSBehaviour(is.T(), client)
	}
	common.CheckInstallationInstallScript(is.T(), client)
	is.testUninstall(client, flavor)
}

func (is *installScriptSuite) IotAgentTest() {
	host := is.Env().RemoteHost
	fileManager := filemanager.NewUnix(host)
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
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
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
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
