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
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var (
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	osVersion             = flag.String("osversion", "", "os version to test")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	platform              = flag.String("platform", "", "platform to test")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	architecture          = flag.String("arch", "x86_64", "architecture to test (x86_64, arm64))")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	flavor                = flag.String("flavor", "datadog-agent", "flavor to test (datadog-agent, datadog-iot-agent, datadog-dogstatsd, datadog-fips-proxy, datadog-heroku-agent)")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	majorVersion          = flag.String("major-version", "7", "major version to test (6, 7)")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

type installScriptSuite struct {
	e2e.BaseSuite[environments.Host]
	cwsSupported bool
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func TestInstallScript(t *testing.T) {
	flake.Mark(t)
	platformJSON := map[string]map[string]map[string]string{}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	err := json.Unmarshal(platforms.Content, &platformJSON)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	osVersions := strings.Split(*osVersion, ",")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	t.Log("Parsed platform json file: ", platformJSON)

	for _, osVers := range osVersions {
		osVers := osVers
		if platformJSON[*platform][*architecture][osVers] == "" {
			// Fail if the image is not defined instead of silently running with default Ubuntu AMI
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			t.Fatalf("No image found for %s %s %s", *platform, *architecture, osVers)
		}

		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		vmOpts := []ec2.VMOption{}
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			vmOpts = append(vmOpts, ec2.WithInstanceType(instanceType))
		}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		t.Run(fmt.Sprintf("test install script on %s %s %s agent %s", osVers, *architecture, *flavor, *majorVersion), func(tt *testing.T) {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			tt.Parallel()
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			tt.Logf("Testing %s", osVers)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			osDesc := platforms.BuildOSDescriptor(*platform, *architecture, osVers)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			vmOpts = append(vmOpts, ec2.WithAMI(platformJSON[*platform][*architecture][osVers], osDesc, osDesc.Architecture))

			e2e.Run(tt,
				&installScriptSuite{cwsSupported: cwsSupported},
				e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
					awshost.WithEC2InstanceOptions(vmOpts...),
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
				)),
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
				e2e.WithStackName(fmt.Sprintf("install-script-test-%v-%v-%s-%s-%v", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *flavor, *majorVersion)),
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
			)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		})
	}
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func (is *installScriptSuite) TestInstallAgent() {
	switch *flavor {
	case "datadog-agent":
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		is.AgentTest("datadog-agent")
	case "datadog-heroku-agent":
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		is.AgentTest("datadog-heroku-agent")
	case "datadog-iot-agent":
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		is.IotAgentTest()
	case "datadog-dogstatsd":
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		is.DogstatsdAgentTest()
	}
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func (is *installScriptSuite) testUninstall(client *common.TestClient, flavor string) {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	is.T().Run("remove the agent", func(tt *testing.T) {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		_, err := client.PkgManager.Remove(flavor)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		require.NoError(tt, err, "should uninstall the agent")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	})

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckUninstallation(is.T(), client)
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func (is *installScriptSuite) AgentTest(flavor string) {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	host := is.Env().RemoteHost
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	fileManager := filemanager.NewUnix(host)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	require.NoError(is.T(), err)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	unixHelper := helpers.NewUnix()
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(flavor), installparams.WithMajorVersion(*majorVersion))

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallation(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckSigningKeys(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentBehaviour(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentStops(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentRestarts(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckIntegrationInstall(is.T(), client)
	if *majorVersion == "6" {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		common.SetAgentPythonMajorVersion(is.T(), client, "2")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		common.CheckAgentPython(is.T(), client, common.ExpectedPythonVersion2)
	}
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.SetAgentPythonMajorVersion(is.T(), client, "3")
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentPython(is.T(), client, common.ExpectedPythonVersion3)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckApmEnabled(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckApmDisabled(is.T(), client)
	if flavor == "datadog-agent" && is.cwsSupported {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
		common.CheckCWSBehaviour(is.T(), client)
	}
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallationInstallScript(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	is.testUninstall(client, flavor)
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func (is *installScriptSuite) IotAgentTest() {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	host := is.Env().RemoteHost
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	fileManager := filemanager.NewUnix(host)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	require.NoError(is.T(), err)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	unixHelper := helpers.NewUnix()
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallation(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckSigningKeys(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentBehaviour(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentStops(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckAgentRestarts(is.T(), client)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallationInstallScript(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	is.testUninstall(client, "datadog-iot-agent")
}

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
func (is *installScriptSuite) DogstatsdAgentTest() {
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	host := is.Env().RemoteHost
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	fileManager := filemanager.NewUnix(host)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	agentClient, err := client.NewHostAgentClient(is.T(), host, false)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	require.NoError(is.T(), err)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	unixHelper := helpers.NewUnixDogstatsd()
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	client := common.NewTestClient(is.Env().RemoteHost, agentClient, fileManager, unixHelper)

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallation(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckSigningKeys(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckDogstatdAgentBehaviour(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckDogstatsdAgentStops(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckDogstatsdAgentRestarts(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	common.CheckInstallationInstallScript(is.T(), client)
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	is.testUninstall(client, "datadog-dogstatsd")
}
