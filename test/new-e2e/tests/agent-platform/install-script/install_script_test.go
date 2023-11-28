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

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	filemanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/file-manager"
	helpers "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/helper"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	e2eOs "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"testing"
)

var osVersion = flag.String("osversion", "", "os version to test")
var platform = flag.String("platform", "", "platform to test")
var cwsSupportedOsVersion = flag.String("cws-supported-osversion", "", "list of os where CWS is supported")
var architecture = flag.String("arch", "x84_64", "architecture to test (x86_64, arm64))")
var flavor = flag.String("flavor", "datadog-agent", "flavor to test (datadog-agent, datadog-iot-agent, datadog-dogstatsd, datadog-fips-proxy, datadog-heroku-agent)")

type installScriptSuite struct {
	e2e.Suite[e2e.VMEnv]
	cwsSupported bool
}

func TestInstallScript(t *testing.T) {
	osMapping := map[string]ec2os.Type{
		"debian":      ec2os.DebianOS,
		"ubuntu":      ec2os.UbuntuOS,
		"centos":      ec2os.CentOS,
		"amazonlinux": ec2os.AmazonLinuxOS,
		"redhat":      ec2os.RedHatOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
	}

	archMapping := map[string]e2eOs.Architecture{
		"x86_64": e2eOs.AMD64Arch,
		"arm64":  e2eOs.ARM64Arch,
	}

	platformJSON := map[string]map[string]map[string]string{}

	err := json.Unmarshal(platforms.Content, &platformJSON)
	require.NoErrorf(t, err, "failed to umarshall platform file: %v", err)

	osVersions := strings.Split(*osVersion, ",")
	cwsSupportedOsVersionList := strings.Split(*cwsSupportedOsVersion, ",")

	fmt.Println("Parsed platform json file: ", platformJSON)
	for _, osVers := range osVersions {
		osVers := osVers
		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}

		t.Run(fmt.Sprintf("test install script on %s %s %s", osVers, *architecture, *flavor), func(tt *testing.T) {
			tt.Parallel()
			fmt.Printf("Testing %s", osVers)
			e2e.Run(tt, &installScriptSuite{cwsSupported: cwsSupported}, e2e.EC2VMStackDef(ec2params.WithImageName(platformJSON[*platform][*architecture][osVers], archMapping[*architecture], osMapping[*platform])), params.WithStackName(fmt.Sprintf("install-script-test-%v-%v-%s-%s", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *flavor)))
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

func (is *installScriptSuite) AgentTest(flavor string) {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)

	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnixHelper()
	client := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(flavor))

	common.CheckInstallation(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)
	common.CheckIntegrationInstall(is.T(), client)
	common.CheckAgentPython(is.T(), client, "3")
	common.CheckApmEnabled(is.T(), client)
	common.CheckApmDisabled(is.T(), client)
	if flavor == "datadog-agent" && is.cwsSupported {
		common.CheckCWSBehaviour(is.T(), client)
	}
	common.CheckInstallationInstallScript(is.T(), client)
	common.CheckUninstallation(is.T(), client, flavor)

}

func (is *installScriptSuite) IotAgentTest() {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)

	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnixHelper()
	client := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

	common.CheckInstallation(is.T(), client)
	common.CheckAgentBehaviour(is.T(), client)
	common.CheckAgentStops(is.T(), client)
	common.CheckAgentRestarts(is.T(), client)

	common.CheckInstallationInstallScript(is.T(), client)
	common.CheckUninstallation(is.T(), client, "datadog-iot-agent")
}

func (is *installScriptSuite) DogstatsdAgentTest() {
	fileManager := filemanager.NewUnixFileManager(is.Env().VM)

	vm := is.Env().VM.(*client.PulumiStackVM)
	agentClient, err := client.NewAgentClient(is.T(), vm, vm.GetOS(), false)
	require.NoError(is.T(), err)

	unixHelper := helpers.NewUnixDogstatsdHelper()
	client := common.NewTestClient(is.Env().VM, agentClient, fileManager, unixHelper)

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(*flavor))

	common.CheckInstallation(is.T(), client)
	common.CheckDogstatdAgentBehaviour(is.T(), client)
	common.CheckDogstatsdAgentStops(is.T(), client)
	common.CheckDogstatsdAgentRestarts(is.T(), client)
	common.CheckInstallationInstallScript(is.T(), client)
	common.CheckUninstallation(is.T(), client, "datadog-dogstatsd")
}
