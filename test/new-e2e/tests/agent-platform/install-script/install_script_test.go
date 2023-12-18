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
var architecture = flag.String("arch", "x86_64", "architecture to test (x86_64, arm64))")
var flavor = flag.String("flavor", "datadog-agent", "flavor to test (datadog-agent, datadog-iot-agent, datadog-dogstatsd, datadog-fips-proxy, datadog-heroku-agent)")
var majorVersion = flag.String("major-version", "7", "major version to test (6, 7)")

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
		"rhel":        ec2os.RedHatOS,
		"sles":        ec2os.SuseOS,
		"windows":     ec2os.WindowsOS,
		"fedora":      ec2os.FedoraOS,
		"suse":        ec2os.SuseOS,
		"rocky":       ec2os.RockyLinux,
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

	t.Log("Parsed platform json file: ", platformJSON)

	for _, osVers := range osVersions {
		vmOpts := []ec2params.Option{}
		osVers := osVers
		if platformJSON[*platform][*architecture][osVers] == "" {
			// Fail if the image is not defined instead of silently running with default Ubuntu AMI
			t.Fatalf("No image found for %s %s %s", *platform, *architecture, osVers)
		}

		var testOsType ec2os.Type
		for osName, osType := range osMapping {
			if strings.Contains(osVers, osName) {
				testOsType = osType
			}
		}

		cwsSupported := false
		for _, cwsSupportedOs := range cwsSupportedOsVersionList {
			if cwsSupportedOs == osVers {
				cwsSupported = true
			}
		}
		vmOpts = append(vmOpts, ec2params.WithImageName(platformJSON[*platform][*architecture][osVers], archMapping[*architecture], testOsType))
		if instanceType, ok := os.LookupEnv("E2E_OVERRIDE_INSTANCE_TYPE"); ok {
			vmOpts = append(vmOpts, ec2params.WithInstanceType(instanceType))
		}
		t.Run(fmt.Sprintf("test install script on %s %s %s agent %s", osVers, *architecture, *flavor, *majorVersion), func(tt *testing.T) {
			tt.Parallel()
			tt.Logf("Testing %s", osVers)
			e2e.Run(tt, &installScriptSuite{cwsSupported: cwsSupported}, e2e.EC2VMStackDef(vmOpts...), params.WithStackName(fmt.Sprintf("install-script-test-%v-%v-%s-%s-%v", os.Getenv("CI_PIPELINE_ID"), osVers, *architecture, *flavor, *majorVersion)))
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

	install.Unix(is.T(), client, installparams.WithArch(*architecture), installparams.WithFlavor(flavor), installparams.WithMajorVersion(*majorVersion))

	common.CheckInstallation(is.T(), client)
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
